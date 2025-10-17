// eastercompany/dex-discord-interface/store/store.go
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bwmarrin/discordgo"
)

// getDexterDataPath constructs the base path for Dexter's data.
func getDexterDataPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory: %w", err)
	}
	return filepath.Join(home, "Dexter", "discord", "server"), nil
}

// SaveMessage saves a message from a server channel to a JSON file.
func SaveMessage(guildID, channelID string, m *discordgo.Message) error {
	basePath, err := getDexterDataPath()
	if err != nil {
		return err
	}

	// Construct the full path for the message logs.
	path := filepath.Join(basePath, guildID, channelID, "messages")
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("could not create directory structure %s: %w", path, err)
	}

	// Marshal the message object into a pretty JSON format.
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal message to JSON: %w", err)
	}

	// Write the JSON data to a file named after the message ID.
	filePath := filepath.Join(path, fmt.Sprintf("%s.json", m.ID))
	return os.WriteFile(filePath, data, 0644)
}

// LogTranscription appends a timestamped transcription to a log file for a specific channel.
func LogTranscription(guildID, channelID, user, transcription string) error {
	basePath, err := getDexterDataPath()
	if err != nil {
		return err
	}

	// Construct the full path for the channel.
	path := filepath.Join(basePath, guildID, channelID)
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("could not create directory structure %s: %w", path, err)
	}

	// Open the transcriptions log file in append mode, creating it if it doesn't exist.
	filePath := filepath.Join(path, "transcriptions.log")
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("could not open transcription log file %s: %w", filePath, err)
	}
	defer file.Close()

	// Format the log entry with a timestamp.
	logEntry := fmt.Sprintf("[%s] %s: %s\n", time.Now().Format(time.RFC3339), user, transcription)

	// Append the new entry to the file.
	if _, err := file.WriteString(logEntry); err != nil {
		return fmt.Errorf("could not write to transcription log file: %w", err)
	}

	return nil
}

