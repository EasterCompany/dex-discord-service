package dashboard

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

// CleanLogChannel deletes all messages in the log channel before initializing dashboards
func CleanLogChannel(session *discordgo.Session, logChannelID string) error {
	log.Println("[CLEANUP] Fetching messages from log channel...")

	messages, err := session.ChannelMessages(logChannelID, 100, "", "", "")
	if err != nil {
		return fmt.Errorf("failed to fetch messages: %w", err)
	}

	if len(messages) == 0 {
		log.Println("[CLEANUP] Log channel is already empty")
		return nil
	}

	log.Printf("[CLEANUP] Deleting %d messages from log channel...\n", len(messages))

	// Try bulk delete first (faster for messages < 14 days old)
	messageIDs := make([]string, len(messages))
	for i, msg := range messages {
		messageIDs[i] = msg.ID
	}

	err = session.ChannelMessagesBulkDelete(logChannelID, messageIDs)
	if err != nil {
		log.Printf("[CLEANUP] Bulk delete failed, falling back to individual deletion: %v\n", err)
		// Fallback: delete individually
		for _, id := range messageIDs {
			if err := session.ChannelMessageDelete(logChannelID, id); err != nil {
				log.Printf("[CLEANUP] Failed to delete message %s: %v\n", id, err)
			}
		}
	}

	log.Printf("[CLEANUP] Successfully cleaned %d messages from log channel\n", len(messages))
	return nil
}
