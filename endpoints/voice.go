package endpoints

import (
	"encoding/json"
	"log"
	"net/http"
)

type VoiceStateRequest struct {
	Mute bool `json:"mute"`
	Deaf bool `json:"deaf"`
}

// VoiceStateHandler updates the bot's voice state (mute/deaf) in the current guild
func VoiceStateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionMutex.RLock()
	dg := discordSession
	sessionMutex.RUnlock()

	vc := GetActiveVoiceConnection()

	if dg == nil {
		http.Error(w, "Discord session not initialized", http.StatusServiceUnavailable)
		return
	}

	// We can only update voice state if we are connected to a voice channel
	if vc == nil {
		// Log warning but return OK to avoid breaking callers who expect success if bot is just offline
		log.Printf("Warning: Voice state update requested but no active voice connection.")
		w.WriteHeader(http.StatusOK)
		return
	}

	var req VoiceStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// ChannelVoiceJoin handles state updates (mute/deaf) if already in channel
	_, err := dg.ChannelVoiceJoin(vc.GuildID, vc.ChannelID, req.Mute, req.Deaf)
	if err != nil {
		log.Printf("Error updating voice state (Mute: %v, Deaf: %v): %v", req.Mute, req.Deaf, err)
		http.Error(w, "Failed to update voice state", http.StatusInternalServerError)
		return
	}

	log.Printf("Voice state updated: Mute=%v, Deaf=%v", req.Mute, req.Deaf)
	w.WriteHeader(http.StatusOK)
}
