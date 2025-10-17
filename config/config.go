package config

import (
	"encoding/json"
	"os"
)

// Config stores the configuration for the bot
type Config struct {
	Token string `json:"token"`
}

// LoadConfig loads the configuration from a file
func LoadConfig(path string) (*Config, error) {
	config := &Config{}

	// First, try to load from environment variable
	if token := os.Getenv("DISCORD_TOKEN"); token != "" {
		config.Token = token
		return config, nil
	}

	// If not in env, try to load from file
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(config)
	if err != nil {
		return nil, err
	}

	return config, nil
}