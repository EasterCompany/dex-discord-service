// eastercompany/dex-discord-interface/config/config_test.go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestEnvironment creates a temporary directory structure for config files.
// It returns the path to the temporary Dexter config directory and a cleanup function.
func setupTestEnvironment(t *testing.T) (string, func()) {
	tempDir, err := os.MkdirTemp("", "dexter-config-test")
	require.NoError(t, err)

	dexterConfigPath := filepath.Join(tempDir, "Dexter", "config")
	err = os.MkdirAll(dexterConfigPath, 0755)
	require.NoError(t, err)

	// Temporarily override the user home directory function to point to our temp dir.
	originalHomeDirFunc := os.UserHomeDir
	osUserHomeDir = func() (string, error) {
		return tempDir, nil
	}

	cleanup := func() {
		osUserHomeDir = originalHomeDirFunc
		os.RemoveAll(tempDir)
	}

	return dexterConfigPath, cleanup
}

// Re-assign os.UserHomeDir to a variable so we can mock it in tests.
var osUserHomeDir = os.UserHomeDir

func TestLoadAllConfigs_Success(t *testing.T) {
	dexterPath, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// --- Create mock config files ---
	// Main config
	mainCfg := MainConfig{DiscordConfig: "discord.json", RedisConfig: "redis.json"}
	mainData, _ := json.Marshal(mainCfg)
	err := os.WriteFile(filepath.Join(dexterPath, "config.json"), mainData, 0644)
	require.NoError(t, err)

	// Discord config
	discordCfg := DiscordConfig{Token: "test-token", LogChannelID: "123"}
	discordData, _ := json.Marshal(discordCfg)
	err = os.WriteFile(filepath.Join(dexterPath, "discord.json"), discordData, 0644)
	require.NoError(t, err)

	// Redis config
	redisCfg := RedisConfig{Addr: "localhost:1234"}
	redisData, _ := json.Marshal(redisCfg)
	err = os.WriteFile(filepath.Join(dexterPath, "redis.json"), redisData, 0644)
	require.NoError(t, err)

	// --- Run the function ---
	allConfig, err := LoadAllConfigs()

	// --- Assert results ---
	assert.NoError(t, err)
	require.NotNil(t, allConfig)
	assert.Equal(t, "test-token", allConfig.Discord.Token)
	assert.Equal(t, "123", allConfig.Discord.LogChannelID)
	assert.Equal(t, "localhost:1234", allConfig.Redis.Addr)
}

func TestLoadAllConfigs_FileCreation(t *testing.T) {
	dexterPath, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// --- Run the function without any pre-existing files ---
	allConfig, err := LoadAllConfigs()

	// --- Assert results ---
	assert.NoError(t, err)
	require.NotNil(t, allConfig)

	// Check that the default files were created
	assert.FileExists(t, filepath.Join(dexterPath, "config.json"))
	assert.FileExists(t, filepath.Join(dexterPath, "discord.json"))
	assert.FileExists(t, filepath.Join(dexterPath, "redis.json"))

	// Check that the config struct has the default values
	assert.Equal(t, "", allConfig.Discord.Token)
	assert.Equal(t, "localhost:6379", allConfig.Redis.Addr)
}

func TestLoadAllConfigs_InvalidJSON(t *testing.T) {
	dexterPath, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Create a malformed JSON file
	err := os.WriteFile(filepath.Join(dexterPath, "config.json"), []byte("{ not valid json }"), 0644)
	require.NoError(t, err)

	// --- Run the function ---
	_, err = LoadAllConfigs()

	// --- Assert results ---
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not decode config file")
}
