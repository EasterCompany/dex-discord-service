// eastercompany/dex-discord-interface/config/new_config.go
package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type DiscordConfig struct {
	Token        string `json:"token"`
	LogServerID  string `json:"log_server_id"`
	LogChannelID string `json:"log_channel_id"`
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

	discordConfig, err := loadDiscordConfig(filepath.Join(dexterPath, "discord.json"))
	if err != nil {
		return nil, err
	}

	redisConfig, err := loadRedisConfig(filepath.Join(dexterPath, "redis.json"))
	if err != nil {
		return nil, err
	}

	return &AllConfig{
		Discord: discordConfig,
		Redis:   redisConfig,
	}, nil
}

func loadDiscordConfig(path string) (*DiscordConfig, error) {
	config := &DiscordConfig{}
	return config, loadOrCreate(path, config, &DiscordConfig{Token: "", LogServerID: "", LogChannelID: ""})
}

func loadRedisConfig(path string) (*RedisConfig, error) {
	config := &RedisConfig{}
	return config, loadOrCreate(path, config, &RedisConfig{Addr: "localhost:6379"})
}

func loadOrCreate(path string, v interface{}, defaultConfig interface{}) error {
	log.Printf("Loading config from: %s", path)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return fmt.Errorf("could not create directory for config file at %s: %w", path, err)
			}
			if err := createDefaultConfig(path, defaultConfig); err != nil {
				return err
			}
			data, _ := json.Marshal(defaultConfig)
			return json.Unmarshal(data, v)
		}
		return fmt.Errorf("could not open config file at %s: %w", path, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("could not decode config file at %s: %w", path, err)
	}

	return nil
}

func createDefaultConfig(path string, defaultConfig interface{}) error {
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
