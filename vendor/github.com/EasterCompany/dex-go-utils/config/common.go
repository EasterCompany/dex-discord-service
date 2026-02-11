package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// GetConfigPath returns the absolute path to a configuration file in ~/Dexter/config
func GetConfigPath(filename string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get home directory: %w", err)
	}
	return filepath.Join(home, "Dexter", "config", filename), nil
}

// LoadConfig loads a configuration file from ~/Dexter/config
func LoadConfig[T any](filename string) (*T, error) {
	path, err := GetConfigPath(filename)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg T
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s: %w", filename, err)
	}

	return &cfg, nil
}

// SaveConfig saves a configuration object to ~/Dexter/config
func SaveConfig[T any](filename string, cfg T) error {
	path, err := GetConfigPath(filename)
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", filename, err)
	}

	return os.WriteFile(path, data, 0644)
}

// LoadOrInitConfig loads a configuration file or initializes it with defaults if missing
func LoadOrInitConfig[T any](filename string, defaults T) (*T, error) {
	cfg, err := LoadConfig[T](filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := SaveConfig(filename, defaults); err != nil {
				return nil, err
			}
			return &defaults, nil
		}
		return nil, err
	}
	return cfg, nil
}
