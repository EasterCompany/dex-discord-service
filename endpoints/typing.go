package endpoints

import (
	"encoding/json"
	"log"
	"net/http"
)

type TypingRequest struct {
	ChannelID string `json:"channel_id"`
}

// TypingHandler triggers the "typing..." indicator in a Discord channel
func TypingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if discordSession == nil {
		http.Error(w, "Discord session not ready", http.StatusServiceUnavailable)
		return
	}

	var req TypingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.ChannelID == "" {
		http.Error(w, "channel_id is required", http.StatusBadRequest)
		return
	}

	if err := discordSession.ChannelTyping(req.ChannelID); err != nil {
		log.Printf("Error sending typing indicator to channel %s: %v", req.ChannelID, err)
		http.Error(w, "Failed to trigger typing indicator", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
