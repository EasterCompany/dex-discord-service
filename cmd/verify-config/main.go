package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/EasterCompany/dex-discord-interface/interfaces"
)

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
)

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
				BotConfig     string `json:"bot_config"`
				GcloudConfig  string `json:"gcloud_config"`
				PersonaConfig string `json:"persona_config"`
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
			}{},
		},
		{
			FileName: "cache.json",
			Path:     filepath.Join(configDir, "cache.json"),
			Model: struct {
				Local struct {
					Addr     string `json:"addr"`
					Username string `json:"username"`
					Password string `json:"password"`
					DB       int    `json:"db"`
				} `json:"local"`
				Cloud struct {
					Addr     string `json:"addr"`
					Username string `json:"username"`
					Password string `json:"password"`
					DB       int    `json:"db"`
				} `json:"cloud"`
			}{},
		},
		{
			FileName: "bot.json",
			Path:     filepath.Join(configDir, "bot.json"),
			Model: struct {
				VoiceTimeoutSeconds int    `json:"voice_timeout_seconds"`
				AudioTTLMinutes     int    `json:"audio_ttl_minutes"`
				EngagementModel     string `json:"engagement_model"`
				ConversationalModel string `json:"conversational_model"`
			}{},
		},
		{
			FileName: "persona.json",
			Path:     filepath.Join(configDir, "persona.json"),
			Model:    interfaces.Persona{},
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
	content, err := os.ReadFile(schema.Path)
	if err != nil {
		defaultPath := strings.Replace(schema.Path, ".json", ".default.json", 1)
		content, err = os.ReadFile(defaultPath)
		if err != nil {
			fmt.Printf("  %s[FAIL]%s File not found or not readable (and no .default.json fallback): %v\n", ColorRed, ColorReset, err)
			return false
		}
		fmt.Printf("  %s[OK]%s File exists (using .default.json fallback) and is readable.\n", ColorGreen, ColorReset)
	} else {
		fmt.Printf("  %s[OK]%s File exists and is readable.\n", ColorGreen, ColorReset)
	}

	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()

	modelType := reflect.TypeOf(schema.Model)
	modelInstance := reflect.New(modelType).Interface()

	if err := decoder.Decode(modelInstance); err != nil {
		fmt.Printf("  %s[FAIL]%s JSON is invalid or contains unexpected fields: %v\n", ColorRed, ColorReset, err)
		return false
	}
	fmt.Printf("  %s[OK]%s JSON is valid and all fields are recognized.\n", ColorGreen, ColorReset)

	val := reflect.ValueOf(modelInstance).Elem()
	typ := val.Type()
	missingFields := []string{}
	checkFields(val, typ, &missingFields, "")

	if len(missingFields) > 0 {
		fmt.Printf("  %s[WARN]%s The following fields are present but have empty/default values: %v\n", ColorYellow, ColorReset, missingFields)
	} else {
		fmt.Printf("  %s[OK]%s All required fields have non-empty values.\n", ColorGreen, ColorReset)
	}

	return true
}

func checkFields(val reflect.Value, typ reflect.Type, missingFields *[]string, prefix string) {
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		if fieldType.PkgPath != "" {
			continue
		}

		if field.Kind() == reflect.Struct {
			checkFields(field, field.Type(), missingFields, prefix+fieldType.Name+".")
			continue
		}

		if field.IsZero() {
			*missingFields = append(*missingFields, prefix+fieldType.Name)
		}
	}
}
