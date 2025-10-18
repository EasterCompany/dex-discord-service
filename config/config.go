package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	logger "github.com/EasterCompany/dex-discord-interface/log"
)

// MainConfig points to the other config files.
type MainConfig struct {
	DiscordConfigPath string `json:"discord_config"`
	CacheConfigPath   string `json:"cache_config"`
}

// DiscordConfig holds all discord-related configuration.
type DiscordConfig struct {
	Token                  string `json:"token"`
	HomeServerID           string `json:"home_server_id"`
	LogChannelID           string `json:"log_channel_id"`
	TranscriptionChannelID string `json:"transcription_channel_id"`
	AudioTTLDays           int    `json:"audio_ttl_days"`
}

// CacheConfig holds the configurations for cache connections.
type CacheConfig struct {
	Local *ConnectionConfig `json:"local"`
	Cloud *ConnectionConfig `json:"cloud"`
}

// ConnectionConfig holds the details for a single cache connection.
type ConnectionConfig struct {
	Addr     string `json:"addr"`
	Username string `json:"username"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

// AllConfig holds all configuration for the application.
type AllConfig struct {
	Discord *DiscordConfig
	Cache   *CacheConfig
}

func getConfigPath(filename string) (string, error) {
	if strings.HasPrefix(filename, "/") {
		return filename, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory: %w", err)
	}
	return filepath.Join(home, "Dexter", "config", filename), nil
}

func LoadAllConfigs() (*AllConfig, error) {
	mainConfigPath, err := getConfigPath("config.json")
	if err != nil {
		return nil, err
	}

	mainConfig := &MainConfig{}
	if err := loadOrCreate(mainConfigPath, mainConfig, &MainConfig{
		DiscordConfigPath: "discord.json",
		CacheConfigPath:   "cache.json",
	}); err != nil {
		return nil, fmt.Errorf("could not load main config: %w", err)
	}

	discordConfigPath, err := getConfigPath(mainConfig.DiscordConfigPath)
	if err != nil {
		return nil, err
	}
	discordConfig := &DiscordConfig{}
	if err := loadOrCreate(discordConfigPath, discordConfig, &DiscordConfig{
		Token:                  "",
		HomeServerID:           "",
		LogChannelID:           "",
		TranscriptionChannelID: "",
		AudioTTLDays:           7, // Default to a 7-day TTL for audio files
	}); err != nil {
		return nil, fmt.Errorf("could not load discord config: %w", err)
	}

	cacheConfigPath, err := getConfigPath(mainConfig.CacheConfigPath)
	if err != nil {
		return nil, err
	}
	cacheConfig := &CacheConfig{}
	if err := loadOrCreate(cacheConfigPath, cacheConfig, &CacheConfig{
		Local: &ConnectionConfig{Addr: "localhost:6379", Username: "", Password: "", DB: 0},
		Cloud: &ConnectionConfig{Addr: "", Username: "", Password: "", DB: 0},
	}); err != nil {
		return nil, fmt.Errorf("could not load cache config: %w", err)
	}

	return &AllConfig{
		Discord: discordConfig,
		Cache:   cacheConfig,
	}, nil
}

func loadOrCreate(path string, v interface{}, defaultConfig interface{}) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Post(fmt.Sprintf("Config file not found at `%s`. Creating a default one.", path))
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return fmt.Errorf("could not create directory for config file at %s: %w", path, err)
			}
			return createDefaultConfig(path, defaultConfig)
		}
		return fmt.Errorf("could not open config file at %s: %w", path, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(v); err != nil {
		backupPath := path + ".bak." + time.Now().Format("20060102150405")
		logger.Post(fmt.Sprintf("Failed to decode config `%s`, it might be outdated. Backing up to `%s` and creating a new default.", path, backupPath))
		if err := os.Rename(path, backupPath); err != nil {
			return fmt.Errorf("failed to backup outdated config at %s: %w", path, err)
		}
		return createDefaultConfig(path, defaultConfig)
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
	return encoder.Encode(defaultConfig)
}
