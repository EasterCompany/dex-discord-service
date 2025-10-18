package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

// ANSI color codes for formatted output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
)

// Represents the expected structure of a config file for validation.
type ConfigSchema struct {
	FileName string
	Path     string
	Model    interface{}
}

func main() {
	fmt.Printf("%s--- Dexter Config Verifier ---%s\n", ColorBlue, ColorReset)

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("%s[FATAL]%s Could not determine user home directory: %v\n", ColorRed, ColorReset, err)
		os.Exit(1)
	}
	configDir := filepath.Join(home, "Dexter", "config")

	schemas := []ConfigSchema{
		{
			FileName: "config.json",
			Path:     filepath.Join(configDir, "config.json"),
			Model: struct {
				DiscordConfig string `json:"discord_config"`
				CacheConfig   string `json:"cache_config"`
			}{},
		},
		{
			FileName: "discord.json",
			Path:     filepath.Join(configDir, "discord.json"),
			Model: struct {
				Token                  string `json:"token"`
				HomeServerID           string `json:"home_server_id"`
				LogChannelID           string `json:"log_channel_id"`
				TranscriptionChannelID string `json:"transcription_channel_id"`
				AudioTTLMinutes        int    `json:"audio_ttl_minutes"`
			}{},
		},
		{
			FileName: "cache.json",
			Path:     filepath.Join(configDir, "cache.json"),
			Model: struct {
				Local map[string]interface{} `json:"local"`
				Cloud map[string]interface{} `json:"cloud"`
			}{},
		},
	}

	allChecksPassed := true
	for _, schema := range schemas {
		fmt.Printf("\nVerifying %s'%s'%s...\n", ColorBlue, schema.FileName, ColorReset)
		ok := verifyConfigFile(schema)
		if !ok {
			allChecksPassed = false
		}
	}

	fmt.Println("\n--------------------------")
	if allChecksPassed {
		fmt.Printf("%s✅ All configuration files seem correct.%s\n", ColorGreen, ColorReset)
	} else {
		fmt.Printf("%s❌ Some issues were found in the configuration.%s\n", ColorRed, ColorReset)
		os.Exit(1)
	}
}

func verifyConfigFile(schema ConfigSchema) bool {
	// 1. Check file existence
	content, err := os.ReadFile(schema.Path)
	if err != nil {
		fmt.Printf("  %s[FAIL]%s File not found or not readable: %v\n", ColorRed, ColorReset, err)
		return false
	}
	fmt.Printf("  %s[OK]%s File exists and is readable.\n", ColorGreen, ColorReset)

	// 2. Check for valid JSON and unknown fields
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields() // This is the key check for extra fields

	// We create a new instance of the model struct for decoding.
	modelType := reflect.TypeOf(schema.Model)
	modelInstance := reflect.New(modelType).Interface()

	if err := decoder.Decode(modelInstance); err != nil {
		fmt.Printf("  %s[FAIL]%s JSON is invalid or contains unexpected fields: %v\n", ColorRed, ColorReset, err)
		return false
	}
	fmt.Printf("  %s[OK]%s JSON is valid and all fields are recognized.\n", ColorGreen, ColorReset)

	// 3. Check for missing fields (by checking if any field has its zero value)
	// This is a simple check; more complex validation could be added here.
	val := reflect.ValueOf(modelInstance).Elem()
	typ := val.Type()
	missingFields := []string{}
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		if field.IsZero() {
			missingFields = append(missingFields, typ.Field(i).Name)
		}
	}

	if len(missingFields) > 0 {
		fmt.Printf("  %s[WARN]%s The following fields are present but have empty/default values: %v\n", ColorYellow, ColorReset, missingFields)
		// This is a warning, not a failure, so we still return true.
	} else {
		fmt.Printf("  %s[OK]%s All required fields have non-empty values.\n", ColorGreen, ColorReset)
	}

	return true
}
