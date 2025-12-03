package endpoints

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/EasterCompany/dex-discord-service/audio"
)

// AudioHandler handles requests for audio files from Redis
func AudioHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract Redis key from the URL path
	redisKey := strings.TrimPrefix(r.URL.Path, "/audio/")
	if redisKey == "" {
		http.Error(w, "Missing audio key", http.StatusBadRequest)
		return
	}

	// Get the voice recorder instance
	voiceRecorder, err := audio.NewVoiceRecorder(context.Background(), nil, nil)
	if err != nil {
		log.Printf("Error creating voice recorder: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Get audio data from Redis
	audioData, err := voiceRecorder.GetRedis().Get(context.Background(), redisKey).Bytes()
	if err != nil {
		log.Printf("Error getting audio from Redis: %v", err)
		http.Error(w, "Audio not found", http.StatusNotFound)
		return
	}

	// Set headers and write audio data to the response
	w.Header().Set("Content-Type", "audio/wav")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(audioData); err != nil {
		log.Printf("Error writing audio data to response: %v", err)
	}
}
