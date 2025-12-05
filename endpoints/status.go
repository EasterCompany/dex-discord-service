package endpoints

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/bwmarrin/discordgo"
)

type StatusUpdateRequest struct {
	ActivityType int    `json:"activity_type"` // 0: Game, 1: Streaming, 2: Listening, 3: Watching, 5: Competing
	StatusText   string `json:"status_text"`   // The text to display
	OnlineStatus string `json:"online_status"` // online, idle, dnd, invisible
}

// UpdateStatusHandler updates the bot's presence/status on Discord
func UpdateStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if discordSession == nil {
		http.Error(w, "Discord session not ready", http.StatusServiceUnavailable)
		return
	}

	var req StatusUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// Default to "Listening" (2) if not specified, or 0 (Game) if text is set?
	// Actually, let's trust the caller. If Type is 0, it's "Playing ...".
	// Common pattern:
	// Thinking -> "Watching inputs..." (3) or just "Playing Thinking..."
	// Let's default to Game (0) if type is not provided but text is?
	// JSON unmarshal defaults int to 0.

	// Set the status
	// UpdateStatusComplex(idle int, gameType int, gameName string, url string) (err error)
	// idle: json boolean/int? DiscordGo uses UpdateStatusComplex which takes:
	// usd *UpdateStatusData.

	// Ideally we use s.UpdateStatusComplex for full control, but s.UpdateGameStatus is simpler.
	// s.UpdateGameStatus(idle, name) -> type 0.
	// s.UpdateListeningStatus(name) -> type 2.

	// Let's use UpdateStatusComplex manually constructed.
	// ActivityType: https://discord.com/developers/docs/topics/gateway-events#activity-object-activity-types
	// 0 Game, 1 Streaming, 2 Listening, 3 Watching, 5 Competing

	err := discordSession.UpdateStatusComplex(discordgo.UpdateStatusData{
		Status: req.OnlineStatus, // "online", "dnd", "idle", "invisible"
		Activities: []*discordgo.Activity{
			{
				Name: req.StatusText,
				Type: discordgo.ActivityType(req.ActivityType),
			},
		},
	})

	if err != nil {
		log.Printf("Error updating status: %v", err)
		http.Error(w, "Failed to update status", http.StatusInternalServerError)
		return
	}

	log.Printf("Bot status updated: [%s] %s (Type: %d)", req.OnlineStatus, req.StatusText, req.ActivityType)
	w.WriteHeader(http.StatusOK)
}
