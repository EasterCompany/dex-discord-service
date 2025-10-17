package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bwmarrin/discordgo"
)

func getHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home, nil
}

// SaveServerMessage saves a message from a server
func SaveServerMessage(serverName, channelName string, m *discordgo.Message) error {
	home, err := getHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, "Dexter/Discord/Servers", serverName, channelName)
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(path, fmt.Sprintf("%s.json", m.ID))

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

// SaveDirectMessage saves a direct message
func SaveDirectMessage(userID string, m *discordgo.Message) error {
	home, err := getHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, "Dexter/Discord/Users", userID)
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(path, fmt.Sprintf("%s.json", m.ID))

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}