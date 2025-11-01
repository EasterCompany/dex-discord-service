package handlers

import (
	"fmt"
	"log"
	"strings"

	contextpkg "github.com/EasterCompany/dex-discord-service/context"
	"github.com/EasterCompany/dex-discord-service/dashboard"
	"github.com/bwmarrin/discordgo"
)

// MessageCreateHandler handles new messages and updates the dashboard.
func MessageCreateHandler(
	d *dashboard.MessagesDashboard,
	sm *StatusManager,
	sb *contextpkg.SnapshotBuilder,
) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore messages from the bot itself
		if m.Author.ID == s.State.User.ID {
			return
		}

		// If bot is sleeping, check for trigger words to wake it up
		if sm.GetStatus() == "Sleeping" {
			lowerContent := strings.ToLower(m.Content)
			// Check for wake triggers (optimized - most common first)
			if strings.Contains(lowerContent, "dexter") ||
				strings.Contains(lowerContent, "hey dexter") ||
				strings.Contains(lowerContent, "wake up") {
				sm.SetIdle() // Wake up to idle

				// CAPTURE CONTEXT SNAPSHOT when triggered
				snapshot, err := sb.CaptureSnapshot(
					m.Author.Username,
					m.ChannelID,
					m.Content,
					sm.GetStatus(),
				)
				if err != nil {
					log.Printf("Error capturing context snapshot: %v", err)
				} else {
					// Format for LLM and log it (for now, we'll just log it)
					// When we integrate with LLM service, this is what we send
					contextStr := snapshot.FormatForLLM()
					log.Printf("\n=== CONTEXT SNAPSHOT CAPTURED ===\n%s\n", contextStr)

					// TODO: Send contextStr to LLM service for processing
					// For now, we're just capturing and logging the context
				}
			} else {
				// Still sleeping, don't process message
				return
			}
		}

		// Get channel name
		channel, err := s.State.Channel(m.ChannelID)
		if err != nil {
			// Could be a DM, log it with channel ID
			channel = &discordgo.Channel{Name: m.ChannelID}
		}

		// Format timestamp
		timestamp := m.Timestamp.Format("15:04:05")

		// Truncate message content
		contentPreview := m.Content
		if len(contentPreview) > 50 {
			contentPreview = contentPreview[:50] + "..."
		}

		// Format the log entry
		logEntry := fmt.Sprintf("[%s] %s @ %s: %s",
			channel.Name,
			m.Author.Username,
			timestamp,
			contentPreview,
		)

		// Add message to the dashboard
		d.AddMessage(logEntry)
	}
}
