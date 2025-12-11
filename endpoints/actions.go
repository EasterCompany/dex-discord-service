package endpoints

import (
	"encoding/json"
	"log"
	"net/http"
)

// DeleteMessageRequest represents the structure of a delete message request
type DeleteMessageRequest struct {
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
}

// DeleteMessageHandler handles requests to delete a message
func DeleteMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionMutex.RLock()
	dg := discordSession
	sessionMutex.RUnlock()

	if dg == nil {
		http.Error(w, "Discord session not initialized", http.StatusServiceUnavailable)
		return
	}

	var req DeleteMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ChannelID == "" || req.MessageID == "" {
		http.Error(w, "channel_id and message_id are required", http.StatusBadRequest)
		return
	}

	if err := dg.ChannelMessageDelete(req.ChannelID, req.MessageID); err != nil {
		log.Printf("Error deleting message %s in channel %s: %v", req.MessageID, req.ChannelID, err)
		http.Error(w, "Failed to delete message", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully deleted message %s in channel %s", req.MessageID, req.ChannelID)
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]bool{"success": true}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
