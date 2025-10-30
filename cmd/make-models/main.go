package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
)

type BotConfig struct {
	VoiceTimeoutSeconds int    `json:"voice_timeout_seconds"`
	AudioTTLMinutes     int    `json:"audio_ttl_minutes"`
	LLMServerURL        string `json:"llm_server_url"`
	EngagementModel     string `json:"engagement_model"`
	ConversationalModel string `json:"conversational_model"`
}

func main() {
	fmt.Printf("%s--- Dexter Model Maker ---%s\n", ColorBlue, ColorReset)

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("%s[FATAL]%s Could not determine user home directory: %v\n", ColorRed, ColorReset, err)
		os.Exit(1)
	}
	dexterConfigDir := filepath.Join(home, "Dexter", "config")
	botConfigPath := filepath.Join(dexterConfigDir, "bot.json")

	botConfig, err := loadBotConfig(botConfigPath)
	if err != nil {
		fmt.Printf("%s[FATAL]%s Failed to load bot.json: %v\n", ColorRed, ColorReset, err)
		os.Exit(1)
	}

	ollamaModelsDir := filepath.Join(home, "Dexter", "models")
	if err := os.MkdirAll(ollamaModelsDir, 0755); err != nil {
		fmt.Printf("%s[FATAL]%s Failed to create Ollama models directory: %v\n", ColorRed, ColorReset, err)
		os.Exit(1)
	}

	// Process engagement model
	fmt.Printf("\n%sVerifying engagement model '%s'%s...\n", ColorBlue, botConfig.EngagementModel, ColorReset)
	engagementModelfilePath := filepath.Join(ollamaModelsDir, "Modelfile-engagement")
	if err := processModel("dexter-engagement", botConfig.EngagementModel, engagementModelfilePath, getEngagementSystemPrompt(), getEngagementParameters()); err != nil {
		fmt.Printf("%s[ERROR]%s Failed to process engagement model: %v\n", ColorRed, ColorReset, err)
	} else {
		fmt.Printf("%s[SUCCESS]%s Engagement model 'dexter-engagement' processed.\n", ColorGreen, ColorReset)
	}

	// Process conversational model
	fmt.Printf("\n%sVerifying conversational model '%s'%s...\n", ColorBlue, botConfig.ConversationalModel, ColorReset)
	conversationalModelfilePath := filepath.Join(ollamaModelsDir, "Modelfile-conversation")
	if err := processModel("dexter-conversation", botConfig.ConversationalModel, conversationalModelfilePath, getConversationalSystemPrompt(), getConversationalParameters()); err != nil {
		fmt.Printf("%s[ERROR]%s Failed to process conversational model: %v\n", ColorRed, ColorReset, err)
	} else {
		fmt.Printf("%s[SUCCESS]%s Conversational model 'dexter-conversation' processed.\n", ColorGreen, ColorReset)
	}

	fmt.Printf("\n%s--- Model Maker Finished ---%s\n", ColorBlue, ColorReset)
}

func loadBotConfig(path string) (*BotConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read bot config at %s: %w", path, err)
	}
	var config BotConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse bot config at %s: %w", path, err)
	}
	return &config, nil
}

func processModel(newModelName, baseModel, modelfilePath, systemPrompt string, parameters map[string]string) error {
	fmt.Printf("  %s[INFO]%s Pulling base model '%s' (if not already present or up-to-date)...\n", ColorBlue, ColorReset, baseModel)
	if err := runOllamaCommand("pull", baseModel); err != nil {
		return fmt.Errorf("failed to pull base model '%s': %w", baseModel, err)
	}
	fmt.Printf("  %s[OK]%s Base model '%s' is ready.\n", ColorGreen, ColorReset, baseModel)

	fmt.Printf("  %s[INFO]%s Generating Modelfile for '%s' at '%s'...\n", ColorBlue, ColorReset, newModelName, modelfilePath)
	modelfileContent := generateModelfile(baseModel, systemPrompt, parameters)
	if err := os.WriteFile(modelfilePath, []byte(modelfileContent), 0644); err != nil {
		return fmt.Errorf("failed to write Modelfile to %s: %w", modelfilePath, err)
	}
	fmt.Printf("  %s[OK]%s Modelfile for '%s' generated.\n", ColorGreen, ColorReset, newModelName)

	fmt.Printf("  %s[INFO]%s Creating Ollama model '%s' from Modelfile...\n", ColorBlue, ColorReset, newModelName)
	if err := runOllamaCommand("create", newModelName, "-f", modelfilePath); err != nil {
		return fmt.Errorf("failed to create Ollama model '%s': %w", newModelName, err)
	}
	fmt.Printf("  %s[OK]%s Ollama model '%s' created/updated.\n", ColorGreen, ColorReset, newModelName)

	return nil
}

func runOllamaCommand(args ...string) error {
	cmd := exec.Command("ollama", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Only include stdout/stderr in error if they contain something
		var errorMsg strings.Builder
		errorMsg.WriteString(fmt.Sprintf("ollama command failed: %v", err))
		if stdout.Len() > 0 {
			errorMsg.WriteString(fmt.Sprintf("\nStdout: %s", stdout.String()))
		}
		if stderr.Len() > 0 {
			errorMsg.WriteString(fmt.Sprintf("\nStderr: %s", stderr.String()))
		}
		return errors.New(errorMsg.String())
	}
	// Print ollama's output to give user feedback
	if stdout.Len() > 0 {
		fmt.Printf("%s", stdout.String())
	}
	if stderr.Len() > 0 {
		fmt.Printf("%s", stderr.String())
	}
	return nil
}

func generateModelfile(baseModel, systemPrompt string, parameters map[string]string) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("FROM %s\n", baseModel))
	for key, value := range parameters {
		builder.WriteString(fmt.Sprintf("PARAMETER %s %s\n", key, value))
	}
	builder.WriteString(fmt.Sprintf("SYSTEM \"\"\"%s\"\"\"\n", systemPrompt))
	return builder.String()
}

func getEngagementSystemPrompt() string {
	return `You are Dexter, a helpful AI assistant for Discord.
You are designed to engage in short, concise conversations.
Keep your responses brief and to the point.
Also be a bit cheeky.`
}

func getEngagementParameters() map[string]string {
	return map[string]string{
		"temperature": "0.7",
	}
}

func getConversationalSystemPrompt() string {
	return `You are Dexter, a highly intelligent and conversational AI assistant for Discord.
You are designed to have extended, natural conversations with users.
Provide detailed and thoughtful responses.
Also be a bit cheeky.`
}

func getConversationalParameters() map[string]string {
	return map[string]string{
		"temperature": "0.8",
	}
}
