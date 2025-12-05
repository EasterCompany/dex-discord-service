package endpoints

import (
	"encoding/json"
	"net/http"

	"github.com/bwmarrin/discordgo"
)

type UserContext struct {
	Username string `json:"username"`
	Status   string `json:"status"`
	Activity string `json:"activity,omitempty"`
}

type ChannelContextResponse struct {
	ChannelName string        `json:"channel_name"`
	GuildName   string        `json:"guild_name,omitempty"`
	Users       []UserContext `json:"users"`
}

// GetChannelContextHandler returns context information about a channel (users, status)
func GetChannelContextHandler(w http.ResponseWriter, r *http.Request) {
	if discordSession == nil {
		http.Error(w, "Discord session not ready", http.StatusServiceUnavailable)
		return
	}

	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		http.Error(w, "channel_id required", http.StatusBadRequest)
		return
	}

	channel, err := discordSession.State.Channel(channelID)
	if err != nil {
		channel, err = discordSession.Channel(channelID)
		if err != nil {
			http.Error(w, "Channel not found", http.StatusNotFound)
			return
		}
	}

	response := ChannelContextResponse{
		ChannelName: channel.Name,
		Users:       []UserContext{},
	}

	if channel.Type == discordgo.ChannelTypeDM || channel.Type == discordgo.ChannelTypeGroupDM {
		response.ChannelName = "DM"
		// For DMs, list recipients
		for _, recipient := range channel.Recipients {
			userCtx := resolveUserStatus(recipient.ID, "", recipient.Username)
			response.Users = append(response.Users, userCtx)
		}
	} else if channel.GuildID != "" {
		// Guild Channel
		guild, err := discordSession.State.Guild(channel.GuildID)
		if err == nil {
			response.GuildName = guild.Name
			// Use Presences to find online/active users
			// This avoids listing thousands of offline members
			for _, p := range guild.Presences {
				// We need the username. Presence struct has User, but sometimes User is incomplete in Presence updates.
				// Try to find member in state to get full user object
				var username string
				if p.User != nil && p.User.Username != "" {
					username = p.User.Username
				} else {
					member, err := discordSession.State.Member(channel.GuildID, p.User.ID)
					if err == nil {
						username = member.User.Username
					} else {
						// Fallback to API if not in state (expensive, but rare if presence exists)
						u, err := discordSession.User(p.User.ID)
						if err == nil {
							username = u.Username
						}
					}
				}

				// Format Activity
				activityStr := ""
				if len(p.Activities) > 0 {
					activityStr = p.Activities[0].Name
					if p.Activities[0].Details != "" {
						activityStr += ": " + p.Activities[0].Details
					} else if p.Activities[0].State != "" {
						activityStr += ": " + p.Activities[0].State
					}
				}

				response.Users = append(response.Users, UserContext{
					Username: username,
					Status:   string(p.Status),
					Activity: activityStr,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func resolveUserStatus(userID, guildID, knownUsername string) UserContext {
	username := knownUsername
	status := "offline"
	activity := ""

	// Try to fetch user if username unknown
	if username == "" {
		user, err := discordSession.User(userID)
		if err == nil {
			username = user.Username
		}
	}

	// Check presence if we are in a guild context, otherwise DM presence is unreliable/unavailable usually
	// But we can check global state if we share a guild with them?
	// For simplicity in DMs, we default to offline or rely on shared guild presence lookups which is complex.
	// Let's just leave as offline for DM unless we find a way.

	// Actually, if we have a session, we can try finding presence in ANY guild we share?
	// Too expensive.

	return UserContext{
		Username: username,
		Status:   status,
		Activity: activity,
	}
}
