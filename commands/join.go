package commands

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// handleJoin handles the /join <channel> command with smart channel matching
func (h *Handler) handleJoin(channelID string, args []string) {
	if len(args) == 0 {
		h.sendResponse(channelID, "❌ Usage: `/join <channel>` - Please specify a voice channel")
		return
	}

	query := strings.Join(args, " ")
	log.Printf("[COMMAND] Searching for voice channel: %s", query)

	// Get all channels in the guild
	channels, err := h.session.GuildChannels(h.config.ServerID)
	if err != nil {
		h.sendResponse(channelID, fmt.Sprintf("❌ Failed to fetch channels: %v", err))
		return
	}

	// Find matching voice channel
	var matches []*discordgo.Channel
	for _, ch := range channels {
		// Only consider voice channels
		if ch.Type != discordgo.ChannelTypeGuildVoice {
			continue
		}

		// Check for exact ID match
		if ch.ID == query {
			matches = []*discordgo.Channel{ch}
			break
		}

		// Check for name match (case-insensitive, partial match)
		if strings.Contains(strings.ToLower(ch.Name), strings.ToLower(query)) {
			matches = append(matches, ch)
		}
	}

	if len(matches) == 0 {
		h.sendResponse(channelID, fmt.Sprintf("❌ No voice channel found matching: `%s`", query))
		return
	}

	if len(matches) > 1 {
		var names []string
		for _, ch := range matches {
			names = append(names, fmt.Sprintf("`%s` (ID: `%s`)", ch.Name, ch.ID))
		}
		h.sendResponse(channelID, fmt.Sprintf("⚠️ Multiple channels found:\n%s\n\nPlease be more specific.", strings.Join(names, "\n")))
		return
	}

	// Single match found
	targetChannel := matches[0]
	h.sendResponse(channelID, fmt.Sprintf("🎵 Joining voice channel: **%s**", targetChannel.Name))

	// Create a task for joining the voice channel
	taskID := h.createTask("join_voice", map[string]interface{}{
		"guild_id":     h.config.ServerID,
		"channel_id":   targetChannel.ID,
		"channel_name": targetChannel.Name,
	})

	h.sendResponse(channelID, fmt.Sprintf("✅ Join task created (ID: `%s`)\nUse `/tasks` to see task queue", taskID))
}
