package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/redis/go-redis/v9"
	"layeh.com/gopus"
)

const (
	channels  int = 2                   // stereo
	frameRate int = 48000               // 48kHz
	frameSize int = 960                 // 20ms frame at 48kHz
	maxBytes  int = (frameSize * 2) * 2 // max size of opus data

	// BargeInThreshold is the RMS energy level required to interrupt the bot.
	// 16-bit audio ranges from -32768 to 32767.
	// A value of 1000 is roughly -30dB, significantly louder than typical acoustic echo.
	BargeInThreshold = 1000.0
)

// UserRecording tracks an active recording session for a user
type UserRecording struct {
	UserID         string
	ChannelID      string
	StartTime      int64
	LastPacketTime int64 // UnixMilli
	Buffer         []int16
	Mutex          sync.Mutex
	Decoder        *gopus.Decoder
}

// VoiceRecorder manages voice recordings for all users
type VoiceRecorder struct {
	recordings       map[string]*UserRecording    // key: userID
	ssrcToUser       map[string]map[uint32]string // maps channelID -> SSRC -> userID
	currentChannelID string                       // currently active channel
	mutex            sync.RWMutex
	redisClient      *redis.Client   // Redis client for storing audio
	ctx              context.Context // Context for Redis operations

	// Callbacks
	OnStart func(userID, channelID string)
	OnStop  func(userID, channelID, redisKey, filePath string)
}

// NewVoiceRecorder creates a new voice recorder instance
func NewVoiceRecorder(ctx context.Context, onStart func(string, string), onStop func(string, string, string, string)) (*VoiceRecorder, error) {
	// Initialize Redis client
	redisClient, err := GetRedisClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Redis client: %w", err)
	}

	vr := &VoiceRecorder{
		recordings:  make(map[string]*UserRecording),
		ssrcToUser:  make(map[string]map[uint32]string),
		redisClient: redisClient,
		ctx:         ctx,
		OnStart:     onStart,
		OnStop:      onStop,
	}

	// Start silence monitor
	go vr.MonitorSilence()

	return vr, nil
}

// MonitorSilence checks for silent users and stops their recordings
func (vr *VoiceRecorder) MonitorSilence() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		vr.mutex.Lock()
		var usersToStop []string
		now := time.Now().UnixMilli()

		for userID, rec := range vr.recordings {
			// Silence threshold: 1500ms (1.5s) to allow natural pauses/thinking time
			if now-rec.LastPacketTime > 1500 {
				usersToStop = append(usersToStop, userID)
			}
		}
		vr.mutex.Unlock()

		for _, userID := range usersToStop {
			// StopRecording handles the mutex internally
			if _, err := vr.StopRecording(userID); err != nil {
				log.Printf("Error stopping recording for user %s: %v", userID, err)
			}
		}
	}
}

// StartRecording begins recording for a user
func (vr *VoiceRecorder) StartRecording(userID, channelID string) error {
	vr.mutex.Lock()
	defer vr.mutex.Unlock()

	// Check if already recording
	if _, exists := vr.recordings[userID]; exists {
		return nil // Already recording
	}

	// Create opus decoder
	decoder, err := gopus.NewDecoder(frameRate, channels)
	if err != nil {
		return fmt.Errorf("failed to create opus decoder: %w", err)
	}

	recording := &UserRecording{
		UserID:         userID,
		ChannelID:      channelID,
		StartTime:      time.Now().Unix(),
		LastPacketTime: time.Now().UnixMilli(),
		Buffer:         make([]int16, 0),
		Decoder:        decoder,
	}

	vr.recordings[userID] = recording

	log.Printf("Started recording for user %s in channel %s", userID, channelID)

	// Trigger callback asynchronously
	if vr.OnStart != nil {
		go vr.OnStart(userID, channelID)
	}

	return nil
}

// StopRecording stops recording for a user and saves the audio to Redis
// Returns the Redis key for the audio data, or empty string if no recording was saved
func (vr *VoiceRecorder) StopRecording(userID string) (string, error) {
	vr.mutex.Lock()
	recording, exists := vr.recordings[userID]
	if !exists {
		vr.mutex.Unlock()
		return "", nil // Not recording
	}
	delete(vr.recordings, userID)
	vr.mutex.Unlock()

	stopTime := time.Now().Unix()

	// Don't save if buffer is empty or recording was too short (< 0.75 second)
	// 48kHz * 2 channels = 96000 samples per second, so 0.75s = 72000
	if len(recording.Buffer) < 72000 {
		log.Printf("Skipping save for user %s: recording too short (%d samples)", userID, len(recording.Buffer))
		// Still trigger stop callback but with empty keys
		if vr.OnStop != nil {
			go vr.OnStop(userID, recording.ChannelID, "", "")
		}
		return "", nil
	}

	// Priority 1: Save to shared disk (Optimal)
	filePath, fileErr := vr.saveRecordingToDisk(recording, stopTime)

	// Priority 2: Save to Redis (Fallback)
	var redisKey string
	var redisErr error

	if fileErr == nil {
		log.Printf("Saved audio to disk for user %s: %s", userID, filePath)
	} else {
		log.Printf("Failed to save audio to disk: %v. Falling back to Redis.", fileErr)
		redisKey, redisErr = vr.saveRecordingToRedis(recording, stopTime)
		if redisErr != nil {
			return "", fmt.Errorf("failed to save recording to Redis (fallback): %w", redisErr)
		}
		log.Printf("Stopped and saved recording for user %s to Redis key %s", userID, redisKey)
	}

	// Trigger callback asynchronously
	if vr.OnStop != nil {
		go vr.OnStop(userID, recording.ChannelID, redisKey, filePath)
	}

	return redisKey, nil
}

// saveRecordingToDisk saves the recorded audio to a local temporary file
func (vr *VoiceRecorder) saveRecordingToDisk(recording *UserRecording, stopTime int64) (string, error) {
	// Create shared directory if it doesn't exist
	tmpDir := "/tmp/dexter/audio"
	if err := os.MkdirAll(tmpDir, 0777); err != nil {
		return "", fmt.Errorf("failed to create temp audio directory: %w", err)
	}

	filename := fmt.Sprintf("%d-%d-%s-%s.wav", recording.StartTime, stopTime, recording.UserID, recording.ChannelID)
	filePath := filepath.Join(tmpDir, filename)

	// Create a buffer to write the WAV file
	var buf bytes.Buffer

	// Write WAV header
	if err := writeWAVHeaderToBuffer(&buf, len(recording.Buffer)); err != nil {
		return "", fmt.Errorf("failed to write WAV header: %w", err)
	}

	// Write PCM data
	if err := binary.Write(&buf, binary.LittleEndian, recording.Buffer); err != nil {
		return "", fmt.Errorf("failed to write audio data: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("failed to write audio file: %w", err)
	}

	return filePath, nil
}

// SetCurrentChannel updates the current channel ID
func (vr *VoiceRecorder) SetCurrentChannel(channelID string) {
	vr.mutex.Lock()
	defer vr.mutex.Unlock()
	vr.currentChannelID = channelID
	log.Printf("Set current voice channel to %s", channelID)
}

// RegisterSSRC maps an SSRC to a user ID for the current channel
func (vr *VoiceRecorder) RegisterSSRC(ssrc uint32, userID string, channelID string) {
	vr.mutex.Lock()
	defer vr.mutex.Unlock()

	// Ensure the channel map exists
	if vr.ssrcToUser[channelID] == nil {
		vr.ssrcToUser[channelID] = make(map[uint32]string)
	}

	vr.ssrcToUser[channelID][ssrc] = userID
	log.Printf("Registered SSRC %d for user %s in channel %s", ssrc, userID, channelID)
}

// UnregisterSSRC removes an SSRC mapping for a specific channel
func (vr *VoiceRecorder) UnregisterSSRC(ssrc uint32, channelID string) {
	vr.mutex.Lock()
	defer vr.mutex.Unlock()

	if channelMap, exists := vr.ssrcToUser[channelID]; exists {
		if userID, exists := channelMap[ssrc]; exists {
			log.Printf("Unregistered SSRC %d for user %s in channel %s", ssrc, userID, channelID)
			delete(channelMap, ssrc)
		}
	}
}

// ClearChannelSSRC clears SSRC mappings for a specific channel
func (vr *VoiceRecorder) ClearChannelSSRC(channelID string) {
	vr.mutex.Lock()
	defer vr.mutex.Unlock()

	if channelMap, exists := vr.ssrcToUser[channelID]; exists {
		count := len(channelMap)
		delete(vr.ssrcToUser, channelID)
		log.Printf("Cleared %d SSRC mappings for channel %s", count, channelID)
	}
}

// StopAllRecordings stops and saves all active recordings (useful when moving channels)
func (vr *VoiceRecorder) StopAllRecordings() {
	vr.mutex.Lock()
	// Get a copy of all user IDs that are currently recording
	userIDs := make([]string, 0, len(vr.recordings))
	for userID := range vr.recordings {
		userIDs = append(userIDs, userID)
	}
	vr.mutex.Unlock()

	// Stop each recording (this will handle the mutex internally)
	for _, userID := range userIDs {
		if _, err := vr.StopRecording(userID); err != nil {
			log.Printf("Error stopping recording for user %s: %v", userID, err)
		}
	}
}

// ProcessVoicePacket processes an incoming voice packet
func (vr *VoiceRecorder) ProcessVoicePacket(ssrc uint32, packet *discordgo.Packet) error {
	// Look up user ID from SSRC in the current channel
	vr.mutex.RLock()
	channelID := vr.currentChannelID
	var userID string
	var exists bool
	if channelMap, ok := vr.ssrcToUser[channelID]; ok {
		userID, exists = channelMap[ssrc]
	}
	vr.mutex.RUnlock()

	if !exists {
		// Unknown SSRC for current channel, skip
		// log.Printf("DEBUG: Unknown SSRC %d for channel %s", ssrc, channelID) // Uncomment for deep debugging
		return nil
	}

	// Check for active recording, start if necessary
	vr.mutex.Lock()
	recording, recordingExists := vr.recordings[userID]
	if !recordingExists {
		// Start new recording (must unlock first to avoid deadlock as StartRecording locks)
		vr.mutex.Unlock()
		if err := vr.StartRecording(userID, channelID); err != nil {
			return fmt.Errorf("failed to auto-start recording: %w", err)
		}
		// Re-acquire the recording
		vr.mutex.Lock()
		recording, recordingExists = vr.recordings[userID]
		if !recordingExists {
			vr.mutex.Unlock()
			return fmt.Errorf("recording failed to start for user %s", userID)
		}
	}

	// Update last packet time
	recording.LastPacketTime = time.Now().UnixMilli()
	vr.mutex.Unlock()

	recording.Mutex.Lock()
	defer recording.Mutex.Unlock()

	// Decode opus to PCM
	pcm, err := recording.Decoder.Decode(packet.Opus, frameSize, false)
	if err != nil {
		return fmt.Errorf("failed to decode opus: %w", err)
	}

	// ECHO CANCELLATION & VAD SCALING
	// If Dexter is currently speaking, we increase the sensitivity threshold.
	// Only loud audio (Barge-In) should pass through. Quiet audio is likely acoustic echo.
	mixer := GetGlobalMixer()
	if mixer != nil && mixer.IsPlaying() {
		rms := calculateRMS(pcm)
		if rms < BargeInThreshold {
			// Drop packet (treat as silence/echo)
			return nil
		}
		// Log interruption for debugging (optional, can be noisy)
		// log.Printf("Barge-In Detected: RMS %.2f > Threshold %.2f", rms, BargeInThreshold)
	}

	// Append to buffer
	recording.Buffer = append(recording.Buffer, pcm...)
	return nil
}

func calculateRMS(pcm []int16) float64 {
	if len(pcm) == 0 {
		return 0
	}
	var sum float64
	for _, sample := range pcm {
		val := float64(sample)
		sum += val * val
	}
	return math.Sqrt(sum / float64(len(pcm)))
}

// saveRecordingToRedis saves the recorded audio to Redis as a WAV file in bytes
// Returns the Redis key where the audio is stored
func (vr *VoiceRecorder) saveRecordingToRedis(recording *UserRecording, stopTime int64) (string, error) {
	// Create a buffer to write the WAV file
	var buf bytes.Buffer

	// Write WAV header
	if err := writeWAVHeaderToBuffer(&buf, len(recording.Buffer)); err != nil {
		return "", fmt.Errorf("failed to write WAV header: %w", err)
	}

	// Write PCM data
	if err := binary.Write(&buf, binary.LittleEndian, recording.Buffer); err != nil {
		return "", fmt.Errorf("failed to write audio data: %w", err)
	}

	// Generate Redis key: discord-audio:{startTime}-{stopTime}-{userID}-{channelID}
	redisKey := fmt.Sprintf("discord-audio:%d-%d-%s-%s", recording.StartTime, stopTime, recording.UserID, recording.ChannelID)

	// Save to Redis with 60 second expiration
	err := vr.redisClient.Set(vr.ctx, redisKey, buf.Bytes(), 60*time.Second).Err()
	if err != nil {
		return "", fmt.Errorf("failed to save to Redis: %w", err)
	}

	duration := float64(stopTime - recording.StartTime)
	log.Printf("Saved audio to Redis: %s (%.2f seconds, %d samples, %d bytes)", redisKey, duration, len(recording.Buffer), buf.Len())
	return redisKey, nil
}

// writeWAVHeaderToBuffer writes a WAV file header to a bytes buffer
func writeWAVHeaderToBuffer(buf *bytes.Buffer, samples int) error {
	// WAV file header
	dataSize := samples * 2 // 16-bit samples
	fileSize := 36 + dataSize

	header := make([]byte, 44)

	// RIFF header
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(fileSize))
	copy(header[8:12], "WAVE")

	// fmt chunk
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)                           // fmt chunk size
	binary.LittleEndian.PutUint16(header[20:22], 1)                            // PCM format
	binary.LittleEndian.PutUint16(header[22:24], uint16(channels))             // channels
	binary.LittleEndian.PutUint32(header[24:28], uint32(frameRate))            // sample rate
	binary.LittleEndian.PutUint32(header[28:32], uint32(frameRate*channels*2)) // byte rate
	binary.LittleEndian.PutUint16(header[32:34], uint16(channels*2))           // block align
	binary.LittleEndian.PutUint16(header[34:36], 16)                           // bits per sample

	// data chunk
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize))

	_, err := buf.Write(header)
	return err
}

// GetActiveRecordings returns the number of active recordings
func (vr *VoiceRecorder) GetActiveRecordings() int {
	vr.mutex.RLock()
	defer vr.mutex.RUnlock()
	return len(vr.recordings)
}

// GetRedis returns the Redis client for external access
func (vr *VoiceRecorder) GetRedis() *redis.Client {
	return vr.redisClient
}
