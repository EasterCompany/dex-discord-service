package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all application configuration
type Config struct {
	DiscordToken  string `json:"discord_token"`
	ServerID      string `json:"server_id"`
	LogChannelID  string `json:"log_channel_id"`
	RedisAddr     string `json:"redis_addr"`
	RedisPassword string `json:"redis_password"`
	RedisDB       int    `json:"redis_db"`
}

// Load reads the configuration from ~/Dexter/config/discord-interface.json
func Load() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, "Dexter", "config", "discord-interface.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate required fields
	if cfg.DiscordToken == "" {
		return nil, fmt.Errorf("discord_token is required")
	}
	if cfg.ServerID == "" {
		return nil, fmt.Errorf("server_id is required")
	}
	if cfg.LogChannelID == "" {
		return nil, fmt.Errorf("log_channel_id is required")
	}

	return &cfg, nil
}
