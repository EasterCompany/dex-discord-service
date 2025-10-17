// eastercompany/dex-discord-interface/config/config.go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config reflects the structure of your config.json file.
type Config struct {
	System struct {
		Google struct {
			CloudAPIKey string `json:"cloud_api_key"`
		} `json:"google"`
		Discord struct {
			Token string `json:"token"`
		} `json:"discord"`
	} `json:"system"`
}

// LoadConfig loads the configuration from the user's Dexter directory.
func LoadConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not get user home directory: %w", err)
	}
	path := filepath.Join(home, "Dexter", "config.json")

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open config file at %s: %w", path, err)
	}
	defer file.Close()

	config := &Config{}
	decoder := json.NewDecoder(file)
	err = decoder.Decode(config)
	if err != nil {
		return nil, fmt.Errorf("could not decode config file: %w", err)
	}

	return config, nil
}

