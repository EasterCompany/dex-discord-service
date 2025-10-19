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
	BotConfigPath     string `json:"bot_config"`
}

// DiscordConfig holds all discord-related configuration.
type DiscordConfig struct {
	Token                  string `json:"token"`
	HomeServerID           string `json:"home_server_id"`
	LogChannelID           string `json:"log_channel_id"`
	TranscriptionChannelID string `json:"transcription_channel_id"`
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

// BotConfig holds all bot-related configuration.
type BotConfig struct {
	VoiceTimeoutSeconds int `json:"voice_timeout_seconds"`
	AudioTTLMinutes     int `json:"audio_ttl_minutes"`
}

// AllConfig holds all configuration for the application.
type AllConfig struct {
	Discord *DiscordConfig
	Cache   *CacheConfig
	Bot     *BotConfig
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
	tmpLogger := logger.NewLogger(nil, "")

	mainConfigPath, err := getConfigPath("config.json")
	if err != nil {
		return nil, err
	}

	mainConfig := &MainConfig{}
	if err := loadOrCreate(mainConfigPath, mainConfig, tmpLogger); err != nil {
		return nil, fmt.Errorf("could not load main config: %w", err)
	}

	discordConfigPath, err := getConfigPath(mainConfig.DiscordConfigPath)
	if err != nil {
		return nil, err
	}
	discordConfig := &DiscordConfig{}
	if err := loadOrCreate(discordConfigPath, discordConfig, tmpLogger); err != nil {
		return nil, fmt.Errorf("could not load discord config: %w", err)
	}

	cacheConfigPath, err := getConfigPath(mainConfig.CacheConfigPath)
	if err != nil {
		return nil, err
	}
	cacheConfig := &CacheConfig{}
	if err := loadOrCreate(cacheConfigPath, cacheConfig, tmpLogger); err != nil {
		return nil, fmt.Errorf("could not load cache config: %w", err)
	}

	botConfigPath, err := getConfigPath(mainConfig.BotConfigPath)
	if err != nil {
		return nil, err
	}
	botConfig := &BotConfig{}
	if err := loadOrCreate(botConfigPath, botConfig, tmpLogger); err != nil {
		return nil, fmt.Errorf("could not load bot config: %w", err)
	}

	return &AllConfig{
		Discord: discordConfig,
		Cache:   cacheConfig,
		Bot:     botConfig,
	}, nil
}

func loadOrCreate(path string, v interface{}, logger logger.Logger) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Post(fmt.Sprintf("Config file not found at `%s`. Creating a default one.", path))
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return fmt.Errorf("could not create directory for config file at %s: %w", path, err)
			}
			return createDefaultConfig(path)
		}
		return fmt.Errorf("could not open config file at %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(v); err != nil {
		backupPath := path + ".bak." + time.Now().Format("20060102150405")
		logger.Post(fmt.Sprintf("Failed to decode config `%s`, it might be outdated. Backing up to `%s` and creating a new default.", path, backupPath))
		if err := os.Rename(path, backupPath); err != nil {
			return fmt.Errorf("failed to backup outdated config at %s: %w", path, err)
		}
		return createDefaultConfig(path)
	}
	return nil
}

func createDefaultConfig(path string) error {
	defaultPath := strings.Replace(path, ".json", ".default.json", 1)
	defaultConfig, err := os.ReadFile(defaultPath)
	if err != nil {
		return fmt.Errorf("could not read default config file at %s: %w", defaultPath, err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create config file at %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	_, err = file.Write(defaultConfig)
	return err
}
