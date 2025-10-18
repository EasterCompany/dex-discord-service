package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type MainConfig struct {
	DiscordConfig string `json:"discord_config"`
	RedisConfig   string `json:"redis_config"`
}

type DiscordConfig struct {
	Token                  string `json:"token"`
	LogServerID            string `json:"log_server_id"`
	LogChannelID           string `json:"log_channel_id"`
	TranscriptionChannelID string `json:"transcription_channel_id"`
}

type RedisConfig struct {
	Addr string `json:"addr"`
}

type AllConfig struct {
	Discord *DiscordConfig
	Redis   *RedisConfig
}

func LoadAllConfigs() (*AllConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not get user home directory: %w", err)
	}
	dexterPath := filepath.Join(home, "Dexter", "config")
	mainConfigPath := filepath.Join(dexterPath, "config.json")

	mainCfg := &MainConfig{}
	if err := loadOrCreate(mainConfigPath, mainCfg, &MainConfig{
		DiscordConfig: "discord.json",
		RedisConfig:   "redis.json",
	}); err != nil {
		return nil, fmt.Errorf("could not load main config: %w", err)
	}

	discordConfig := &DiscordConfig{}
	discordPath := filepath.Join(dexterPath, mainCfg.DiscordConfig)
	if err := loadOrCreate(discordPath, discordConfig, &DiscordConfig{Token: "", LogServerID: "", LogChannelID: "", TranscriptionChannelID: ""}); err != nil {
		return nil, fmt.Errorf("could not load discord config: %w", err)
	}

	redisConfig := &RedisConfig{}
	redisPath := filepath.Join(dexterPath, mainCfg.RedisConfig)
	if err := loadOrCreate(redisPath, redisConfig, &RedisConfig{Addr: "localhost:6379"}); err != nil {
		return nil, fmt.Errorf("could not load redis config: %w", err)
	}

	return &AllConfig{
		Discord: discordConfig,
		Redis:   redisConfig,
	}, nil
}

func loadOrCreate(path string, v interface{}, defaultConfig interface{}) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, so create it with default values.
			fmt.Printf("Config file not found at %s. Creating a default one.\n", path)
			if err := createDefaultConfig(path, defaultConfig); err != nil {
				return err
			}
			// Re-open the file we just created.
			file, err = os.Open(path)
			if err != nil {
				return fmt.Errorf("could not open newly created config file at %s: %w", path, err)
			}
		} else {
			// Another error occurred (e.g., permissions).
			return fmt.Errorf("could not open config file at %s: %w", path, err)
		}
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("could not decode config file at %s: %w", path, err)
	}

	return nil
}

func createDefaultConfig(path string, defaultConfig interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("could not create directory for config file at %s: %w", path, err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create config file at %s: %w", path, err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(defaultConfig); err != nil {
		return fmt.Errorf("could not encode default config to %s: %w", path, err)
	}

	return nil
}
