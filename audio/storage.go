package audio

import (
	"fmt"
	"os"
	"path/filepath"
)

// GetDataDir returns the path to ~/Dexter/data
func GetDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, "Dexter", "data"), nil
}

// EnsureDataDir creates the data directory if it doesn't exist
func EnsureDataDir() error {
	dataDir, err := GetDataDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dataDir, 0755)
}

// GetAudioFilePath generates the full path for an audio file
// Format: ~/Dexter/data/discord-{startTime}-{stopTime}-{userID}-{channelID}.wav
func GetAudioFilePath(startTime, stopTime int64, userID, channelID string) (string, error) {
	dataDir, err := GetDataDir()
	if err != nil {
		return "", err
	}

	filename := fmt.Sprintf("discord-%d-%d-%s-%s.wav", startTime, stopTime, userID, channelID)
	return filepath.Join(dataDir, filename), nil
}
