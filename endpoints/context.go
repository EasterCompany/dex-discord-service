package endpoints

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/EasterCompany/dex-discord-service/utils"
	"github.com/bwmarrin/discordgo"
)

type UserContext struct {
	ID       string `json:"id"`
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

type MemberContext struct {
	ID        string `json:"id"`
	Username  string `json:"username"` // Discord handle
	Nickname  string `json:"nickname"` // Server nickname
	AvatarURL string `json:"avatar_url"`
	Level     string `json:"level"`
	Color     int    `json:"color"` // Decimal color from role
	Status    string `json:"status"`
}

type ContactsResponse struct {
	GuildName string          `json:"guild_name"`
	Members   []MemberContext `json:"members"`
}

// GetContactsHandler returns a list of all resolved guild members with their system levels.
func GetContactsHandler(w http.ResponseWriter, r *http.Request) {
	sessionMutex.RLock()
	dg := discordSession
	roles := roleConfig
	sessionMutex.RUnlock()

	if dg == nil {
		http.Error(w, "Discord session not ready", http.StatusServiceUnavailable)
		return
	}

	// We'll target the primary server ID from config if available, or the first guild
	targetGuildID := r.URL.Query().Get("guild_id")
	if targetGuildID == "" {
		if len(dg.State.Guilds) > 0 {
			targetGuildID = dg.State.Guilds[0].ID
		}
	}

	if targetGuildID == "" {
		http.Error(w, "No guild found", http.StatusNotFound)
		return
	}

	// 1. Check Cache
	cacheKey := fmt.Sprintf("cache:contacts:%s", targetGuildID)
	if redisClient != nil {
		cachedData, err := redisClient.Get(r.Context(), cacheKey).Result()
		if err == nil && cachedData != "" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(cachedData))
			return
		}
	}

	guild, err := dg.State.Guild(targetGuildID)
	if err != nil {
		guild, err = dg.Guild(targetGuildID)
		if err != nil {
			http.Error(w, "Guild not found", http.StatusNotFound)
			return
		}
	}

	// Fetch all members (this can be expensive on huge guilds, but we assume a manageable size)
	members, err := dg.GuildMembers(targetGuildID, "", 1000)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch members: %v", err), http.StatusInternalServerError)
		return
	}

	response := ContactsResponse{
		GuildName: guild.Name,
		Members:   []MemberContext{},
	}

	// Map presences for status
	presences := make(map[string]string)
	for _, p := range guild.Presences {
		presences[p.User.ID] = string(p.Status)
	}

	for _, m := range members {
		level := utils.GetUserLevel(dg, redisClient, targetGuildID, m.User.ID, roles)

		// Determine most prominent role color (highest position with a color)
		memberColor := 0
		highestColorPosition := -1

		for _, roleID := range m.Roles {
			for _, r := range guild.Roles {
				if r.ID == roleID {
					if r.Color != 0 {
						if r.Position > highestColorPosition {
							highestColorPosition = r.Position
							memberColor = r.Color
						}
					}
				}
			}
		}

		status := presences[m.User.ID]
		if status == "" {
			status = "offline"
		}

		nickname := m.Nick
		if nickname == "" {
			nickname = m.User.GlobalName
		}
		if nickname == "" {
			nickname = m.User.Username
		}

		response.Members = append(response.Members, MemberContext{
			ID:        m.User.ID,
			Username:  m.User.Username,
			Nickname:  nickname,
			AvatarURL: m.User.AvatarURL("128"),
			Level:     string(level),
			Color:     memberColor,
			Status:    status,
		})
	}

	// 2. Save to Cache
	if redisClient != nil {
		jsonBytes, err := json.Marshal(response)
		if err == nil {
			redisClient.Set(r.Context(), cacheKey, jsonBytes, 5*time.Minute)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// GetGuildStructureHandler returns the full channel structure of all connected guilds
func GetGuildStructureHandler(w http.ResponseWriter, r *http.Request) {
	sessionMutex.RLock()
	dg := discordSession
	sessionMutex.RUnlock()

	if dg == nil {
		http.Error(w, "Discord session not ready", http.StatusServiceUnavailable)
		return
	}

	var response []GuildStructureResponse

	for _, guild := range dg.State.Guilds {
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
						displayName := utils.GetUserDisplayName(dg, redisClient, guild.ID, vs.UserID)
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
	sessionMutex.RLock()
	dg := discordSession
	sessionMutex.RUnlock()

	if dg == nil {
		http.Error(w, "Discord session not ready", http.StatusServiceUnavailable)
		return
	}

	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		http.Error(w, "channel_id required", http.StatusBadRequest)
		return
	}

	channel, err := dg.State.Channel(channelID)
	if err != nil {
		channel, err = dg.Channel(channelID)
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
		guild, err := dg.State.Guild(channel.GuildID)
		if err == nil {
			response.GuildName = guild.Name
			// Use Presences to find online/active users
			// This avoids listing thousands of offline members
			for _, p := range guild.Presences {
				// Check if user has permission to view this channel
				perms, err := dg.UserChannelPermissions(p.User.ID, channelID)
				if err != nil {
					// If we can't check permissions, skip (safe default)
					continue
				}

				if perms&discordgo.PermissionViewChannel != discordgo.PermissionViewChannel {
					continue
				}

				username := utils.GetUserDisplayName(dg, redisClient, channel.GuildID, p.User.ID)

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
					ID:       p.User.ID,
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

// GetLatestMessageIDHandler returns the ID of the last message in a channel
func GetLatestMessageIDHandler(w http.ResponseWriter, r *http.Request) {
	sessionMutex.RLock()
	dg := discordSession
	sessionMutex.RUnlock()

	if dg == nil {
		http.Error(w, "Discord session not ready", http.StatusServiceUnavailable)
		return
	}

	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		http.Error(w, "channel_id required", http.StatusBadRequest)
		return
	}

	channel, err := dg.State.Channel(channelID)
	if err != nil {
		// Try fetching from API if not in state
		channel, err = dg.Channel(channelID)
		if err != nil {
			http.Error(w, "Channel not found", http.StatusNotFound)
			return
		}
	}

	response := map[string]string{
		"channel_id":      channel.ID,
		"last_message_id": channel.LastMessageID,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// GetMemberHandler returns detailed information about a specific member
func GetMemberHandler(w http.ResponseWriter, r *http.Request) {
	sessionMutex.RLock()
	dg := discordSession
	roles := roleConfig
	sessionMutex.RUnlock()

	if dg == nil {
		http.Error(w, "Discord session not ready", http.StatusServiceUnavailable)
		return
	}

	// Path is /member/{id}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 || parts[2] == "" {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}
	userID := parts[2]

	// Find the guild ID from query param or default to first guild
	targetGuildID := r.URL.Query().Get("guild_id")
	if targetGuildID == "" {
		if len(dg.State.Guilds) > 0 {
			targetGuildID = dg.State.Guilds[0].ID
		}
	}

	if targetGuildID == "" {
		http.Error(w, "No guild found", http.StatusNotFound)
		return
	}

	member, err := dg.State.Member(targetGuildID, userID)
	if err != nil {
		member, err = dg.GuildMember(targetGuildID, userID)
		if err != nil {
			http.Error(w, "Member not found", http.StatusNotFound)
			return
		}
	}

	guild, err := dg.State.Guild(targetGuildID)
	if err != nil {
		guild, _ = dg.Guild(targetGuildID)
	}

	level := utils.GetUserLevel(dg, redisClient, targetGuildID, userID, roles)

	memberColor := 0
	if guild != nil {
		highestColorPosition := -1
		for _, roleID := range member.Roles {
			for _, r := range guild.Roles {
				if r.ID == roleID {
					if r.Color != 0 {
						if r.Position > highestColorPosition {
							highestColorPosition = r.Position
							memberColor = r.Color
						}
					}
				}
			}
		}
	}

	response := MemberContext{
		ID:        userID,
		Username:  utils.GetUserDisplayName(dg, redisClient, targetGuildID, userID),
		AvatarURL: member.User.AvatarURL("128"),
		Level:     string(level),
		Color:     memberColor,
		Status:    "unknown", // Presence is harder to get for single user without searching
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
		ID:       userID,
		Username: username,
		Status:   status,
		Activity: activity,
	}
}
