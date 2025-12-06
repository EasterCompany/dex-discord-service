package endpoints

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/EasterCompany/dex-discord-service/utils"
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

type ChannelInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     int      `json:"type"`
	Position int      `json:"position"`
	Users    []string `json:"users,omitempty"`
}

type CategoryInfo struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Position int           `json:"position"`
	Channels []ChannelInfo `json:"channels"`
}

type GuildStructureResponse struct {
	GuildID       string         `json:"guild_id"`
	GuildName     string         `json:"guild_name"`
	Categories    []CategoryInfo `json:"categories"`
	Uncategorized []ChannelInfo  `json:"uncategorized"`
}

// GetGuildStructureHandler returns the full channel structure of all connected guilds
func GetGuildStructureHandler(w http.ResponseWriter, r *http.Request) {
	if discordSession == nil {
		http.Error(w, "Discord session not ready", http.StatusServiceUnavailable)
		return
	}

	var response []GuildStructureResponse

	for _, guild := range discordSession.State.Guilds {
		structure := GuildStructureResponse{
			GuildID:   guild.ID,
			GuildName: guild.Name,
		}

		categoriesMap := make(map[string]*CategoryInfo)
		var uncategorized []ChannelInfo

		// 1. Find Categories
		for _, c := range guild.Channels {
			if c.Type == discordgo.ChannelTypeGuildCategory {
				categoriesMap[c.ID] = &CategoryInfo{
					ID:       c.ID,
					Name:     c.Name,
					Position: c.Position,
					Channels: []ChannelInfo{},
				}
			}
		}

		// 2. Process Channels
		for _, c := range guild.Channels {
			if c.Type == discordgo.ChannelTypeGuildCategory {
				continue
			}

			// Determine users in voice
			var users []string
			if c.Type == discordgo.ChannelTypeGuildVoice {
				for _, vs := range guild.VoiceStates {
					if vs.ChannelID == c.ID {
						displayName := utils.GetUserDisplayName(discordSession, redisClient, guild.ID, vs.UserID)
						users = append(users, displayName)
					}
				}
			}

			info := ChannelInfo{
				ID:       c.ID,
				Name:     c.Name,
				Type:     int(c.Type),
				Position: c.Position,
				Users:    users,
			}

			if c.ParentID != "" {
				if cat, ok := categoriesMap[c.ParentID]; ok {
					cat.Channels = append(cat.Channels, info)
				} else {
					uncategorized = append(uncategorized, info)
				}
			} else {
				uncategorized = append(uncategorized, info)
			}
		}

		// 3. Convert Map to Slice and Sort
		for _, cat := range categoriesMap {
			// Sort channels in category
			sort.Slice(cat.Channels, func(i, j int) bool {
				return cat.Channels[i].Position < cat.Channels[j].Position
			})
			structure.Categories = append(structure.Categories, *cat)
		}

		// Sort categories
		sort.Slice(structure.Categories, func(i, j int) bool {
			return structure.Categories[i].Position < structure.Categories[j].Position
		})

		// Sort uncategorized
		sort.Slice(uncategorized, func(i, j int) bool {
			return uncategorized[i].Position < uncategorized[j].Position
		})
		structure.Uncategorized = uncategorized

		response = append(response, structure)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
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
				username := utils.GetUserDisplayName(discordSession, redisClient, channel.GuildID, p.User.ID)

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
	username := utils.GetUserDisplayName(discordSession, redisClient, guildID, userID)
	if username == "Unknown User" && knownUsername != "" {
		username = knownUsername
	}

	status := "offline"
	activity := ""

	return UserContext{
		Username: username,
		Status:   status,
		Activity: activity,
	}
}
