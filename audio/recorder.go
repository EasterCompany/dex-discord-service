package audio

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
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
	recordings map[string]*UserRecording // key: userID
	ssrcToUser map[uint32]string         // maps SSRC to userID
	mutex      sync.RWMutex
}

// NewVoiceRecorder creates a new voice recorder instance
func NewVoiceRecorder() *VoiceRecorder {
	return &VoiceRecorder{
		recordings: make(map[string]*UserRecording),
		ssrcToUser: make(map[uint32]string),
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

// StopRecording stops recording for a user and saves the audio file
func (vr *VoiceRecorder) StopRecording(userID string) error {
	vr.mutex.Lock()
	recording, exists := vr.recordings[userID]
	if !exists {
		vr.mutex.Unlock()
		return nil // Not recording
	}
	delete(vr.recordings, userID)
	vr.mutex.Unlock()

	stopTime := time.Now().Unix()

	// Don't save if buffer is empty or recording was too short
	if len(recording.Buffer) == 0 || (stopTime-recording.StartTime) < 1 {
		log.Printf("Skipping save for user %s: recording too short or empty", userID)
		return nil
	}

	// Save the audio file
	if err := vr.saveRecording(recording, stopTime); err != nil {
		return fmt.Errorf("failed to save recording: %w", err)
	}

	log.Printf("Stopped and saved recording for user %s", userID)
	return nil
}

// RegisterSSRC maps an SSRC to a user ID
func (vr *VoiceRecorder) RegisterSSRC(ssrc uint32, userID string) {
	vr.mutex.Lock()
	defer vr.mutex.Unlock()
	vr.ssrcToUser[ssrc] = userID
}

// ProcessVoicePacket processes an incoming voice packet
func (vr *VoiceRecorder) ProcessVoicePacket(ssrc uint32, packet *discordgo.Packet) error {
	// Look up user ID from SSRC
	vr.mutex.RLock()
	userID, exists := vr.ssrcToUser[ssrc]
	vr.mutex.RUnlock()

	if !exists {
		// Unknown SSRC, skip
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

// saveRecording saves the recorded audio to a WAV file
func (vr *VoiceRecorder) saveRecording(recording *UserRecording, stopTime int64) error {
	// Get the full file path
	filePath, err := GetAudioFilePath(recording.StartTime, stopTime, recording.UserID, recording.ChannelID)
	if err != nil {
		return err
	}

	// Create WAV file
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create audio file: %w", err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			log.Printf("Error closing audio file: %v", cerr)
		}
	}()

	// Write WAV header
	if err := writeWAVHeader(file, len(recording.Buffer)); err != nil {
		return fmt.Errorf("failed to write WAV header: %w", err)
	}

	// Write PCM data
	if err := binary.Write(file, binary.LittleEndian, recording.Buffer); err != nil {
		return fmt.Errorf("failed to write audio data: %w", err)
	}

	duration := float64(stopTime - recording.StartTime)
	log.Printf("Saved audio file: %s (%.2f seconds, %d samples)", filepath.Base(filePath), duration, len(recording.Buffer))
	return nil
}

// writeWAVHeader writes a WAV file header
func writeWAVHeader(file *os.File, samples int) error {
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

	_, err := file.Write(header)
	return err
}

// GetActiveRecordings returns the number of active recordings
func (vr *VoiceRecorder) GetActiveRecordings() int {
	vr.mutex.RLock()
	defer vr.mutex.RUnlock()
	return len(vr.recordings)
}
