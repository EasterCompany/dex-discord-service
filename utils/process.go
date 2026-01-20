package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/redis/go-redis/v9"
)

var eventServiceURL string

// SetEventServiceURL sets the base URL for the event service API
func SetEventServiceURL(url string) {
	eventServiceURL = url
}

// ReportProcess updates a process's state via the event service API.
func ReportProcess(ctx context.Context, redisClient *redis.Client, processID string, state string) {
	if eventServiceURL == "" {
		log.Printf("Warning: eventServiceURL not set, cannot report process %s", processID)
		return
	}

	payload := map[string]interface{}{
		"id":    processID,
		"state": state,
		"pid":   os.Getpid(),
	}
	jsonBytes, _ := json.Marshal(payload)

	resp, err := http.Post(eventServiceURL+"/processes", "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		log.Printf("Error reporting process %s: %v", processID, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Event service returned error reporting process %s: %d %s", processID, resp.StatusCode, string(body))
	}
}

// ClearProcess removes a process via the event service API.
func ClearProcess(ctx context.Context, redisClient *redis.Client, processID string) {
	if eventServiceURL == "" {
		return
	}

	req, _ := http.NewRequest(http.MethodDelete, eventServiceURL+"/processes/"+processID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Error clearing process %s: %v", processID, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Event service returned error clearing process %s: %d %s", processID, resp.StatusCode, string(body))
	}
}
