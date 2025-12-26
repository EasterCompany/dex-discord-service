package endpoints

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/EasterCompany/dex-discord-service/config"
	"github.com/bwmarrin/discordgo"
)

var (
	discordSession *discordgo.Session
	sessionMutex   sync.RWMutex
	masterUserID   string
	roleConfig     config.DiscordRoleConfig
)

// SetDiscordSession sets the Discord session for endpoints to use
func SetDiscordSession(s *discordgo.Session) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	discordSession = s
}

// SetUserConfig sets the master user and role mapping for handlers
func SetUserConfig(masterID string, roles config.DiscordRoleConfig) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	masterUserID = masterID
	roleConfig = roles
}

// GetVoiceChannelUserCountHandler handles requests to get the number of users in a voice channel.
func GetVoiceChannelUserCountHandler(w http.ResponseWriter, r *http.Request) {
	channelID := r.URL.Query().Get("id")
	if channelID == "" {
		http.Error(w, "Missing channel ID", http.StatusBadRequest)
		return
	}

	sessionMutex.RLock()
	dg := discordSession
	sessionMutex.RUnlock()

	if dg == nil {
		http.Error(w, "Discord session not initialized", http.StatusServiceUnavailable)
		return
	}

	// We need to find the guild ID for this channel to look up voice states
	channel, err := dg.Channel(channelID)
	if err != nil {
		log.Printf("Error fetching channel %s: %v", channelID, err)
		http.Error(w, "Failed to fetch channel info", http.StatusInternalServerError)
		return
	}

	if channel.GuildID == "" {
		// DM or Group DM - user count logic is different, but for voice context usually implies Guild
		// For DMs, it's just the recipient + bot?
		// Let's assume Guild Voice Channel for now as per requirement context.
		http.Error(w, "Channel is not a guild channel", http.StatusBadRequest)
		return
	}

	guild, err := dg.State.Guild(channel.GuildID)
	if err != nil {
		// Try fetching if not in state
		guild, err = dg.Guild(channel.GuildID)
		if err != nil {
			log.Printf("Error fetching guild %s: %v", channel.GuildID, err)
			http.Error(w, "Failed to fetch guild info", http.StatusInternalServerError)
			return
		}
	}

	userCount := 0
	// Iterate through voice states to count users in this channel
	for _, vs := range guild.VoiceStates {
		if vs.ChannelID == channelID {
			userCount++
		}
	}

	response := map[string]interface{}{
		"channel_id": channelID,
		"user_count": userCount,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
