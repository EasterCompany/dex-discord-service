package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
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
)

// UserRecording tracks an active recording session for a user
type UserRecording struct {
	UserID    string
	ChannelID string
	StartTime int64
	Buffer    []int16
	Mutex     sync.Mutex
	Decoder   *gopus.Decoder
}

// VoiceRecorder manages voice recordings for all users
type VoiceRecorder struct {
	recordings       map[string]*UserRecording    // key: userID
	ssrcToUser       map[string]map[uint32]string // maps channelID -> SSRC -> userID
	currentChannelID string                       // currently active channel
	mutex            sync.RWMutex
	redisClient      *redis.Client   // Redis client for storing audio
	ctx              context.Context // Context for Redis operations
}

// NewVoiceRecorder creates a new voice recorder instance
func NewVoiceRecorder(ctx context.Context) (*VoiceRecorder, error) {
	// Initialize Redis client
	redisClient, err := GetRedisClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Redis client: %w", err)
	}

	return &VoiceRecorder{
		recordings:  make(map[string]*UserRecording),
		ssrcToUser:  make(map[string]map[uint32]string),
		redisClient: redisClient,
		ctx:         ctx,
	}, nil
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
		UserID:    userID,
		ChannelID: channelID,
		StartTime: time.Now().Unix(),
		Buffer:    make([]int16, 0),
		Decoder:   decoder,
	}

	vr.recordings[userID] = recording

	log.Printf("Started recording for user %s in channel %s", userID, channelID)
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

	// Don't save if buffer is empty or recording was too short
	if len(recording.Buffer) == 0 || (stopTime-recording.StartTime) < 1 {
		log.Printf("Skipping save for user %s: recording too short or empty", userID)
		return "", nil
	}

	// Save the audio to Redis
	redisKey, err := vr.saveRecordingToRedis(recording, stopTime)
	if err != nil {
		return "", fmt.Errorf("failed to save recording to Redis: %w", err)
	}

	log.Printf("Stopped and saved recording for user %s to Redis key %s", userID, redisKey)
	return redisKey, nil
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
		return nil
	}
	vr.mutex.RLock()
	recording, exists := vr.recordings[userID]
	vr.mutex.RUnlock()

	if !exists {
		return nil // Not recording for this user
	}

	recording.Mutex.Lock()
	defer recording.Mutex.Unlock()

	// Decode opus to PCM
	pcm, err := recording.Decoder.Decode(packet.Opus, frameSize, false)
	if err != nil {
		return fmt.Errorf("failed to decode opus: %w", err)
	}

	// Append to buffer
	recording.Buffer = append(recording.Buffer, pcm...)
	return nil
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
