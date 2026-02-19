package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

var contextMu sync.Mutex

const ContextLimit = 8192 // Example limit

func summarizeIfNeeded(channelID string, history []interface{}) {
	// Simple heuristic: if we have more than 100 messages, consider it large.
	// A more accurate check would be token count.
	if len(history) < 100 {
		return
	}

	log.Printf("Context Storage: Channel %s reached %d messages. Summarizing...", channelID, len(history))

	// 1. Resolve Model Hub
	// We'll use a hardcoded default for now or resolve from service map if possible
	hubURL := "http://100.100.10.10:8400" // Easter-Server

	// 2. Prepare context text
	var sb bytes.Buffer
	for _, entry := range history {
		entryJSON, _ := json.Marshal(entry)
		sb.Write(entryJSON)
		sb.WriteString("\n")
	}

	prompt := fmt.Sprintf("Summarize the following chat history concisely (max %d tokens):\n\n%s", ContextLimit/10, sb.String())

	reqBody := map[string]interface{}{
		"model":  "dex-summary-model",
		"prompt": prompt,
		"stream": false,
	}
	jsonData, _ := json.Marshal(reqBody)

	resp, err := http.Post(hubURL+"/model/run", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Context Storage: Failed to call summary model: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var result struct {
		Response string `json:"response"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Response != "" {
		// Replace history with summary
		newHistory := []interface{}{
			map[string]interface{}{
				"type":    "context_summary",
				"content": result.Response,
			},
		}

		contextMu.Lock()
		defer contextMu.Unlock()

		home, _ := os.UserHomeDir()
		path := filepath.Join(home, ".local", "data", "discord", "channels", channelID+".json")
		data, _ := json.MarshalIndent(newHistory, "", "  ")
		_ = os.WriteFile(path, data, 0644)
		log.Printf("Context Storage: Channel %s summarized successfully.", channelID)
	}
}

// AppendToChannelContext saves a message to the local channel history JSON.
func AppendToChannelContext(channelID string, entry interface{}) error {
	contextMu.Lock()
	defer contextMu.Unlock()

	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "data", "discord", "channels")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create context directory: %w", err)
	}

	path := filepath.Join(dir, channelID+".json")

	var history []interface{}
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err == nil {
			_ = json.Unmarshal(data, &history)
		}
	}

	history = append(history, entry)

	// Implement summarization logic if history length > limit/2
	go summarizeIfNeeded(channelID, history)

	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func GetChannelContextPath(channelID string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "data", "discord", "channels", channelID+".json")
}
