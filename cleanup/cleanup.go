// Package cleanup provides functions for performing cleanup tasks.
package cleanup

import (
	stdlog "log"
	"strings"

	"github.com/EasterCompany/dex-discord-interface/config"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/bwmarrin/discordgo"
)

type Result struct {
	Name  string
	Count int
}

// ClearChannel removes all messages from a channel, except for a specified message to ignore.
func ClearChannel(s *discordgo.Session, channelID, ignoreMessageID string, discordCfg *config.DiscordConfig, log logger.Logger) Result {
	if channelID == "" {
		return Result{Name: "ClearChannelSkipped", Count: 0}
	}
	messages, err := s.ChannelMessages(channelID, 100, "", "", "")
	if err != nil {
		log.Error("Failed to get messages for channel cleanup", err)
		return Result{Name: "ClearChannelFailed", Count: 0}
	}

	var messageIDs []string
	for _, msg := range messages {
		if msg.ID != ignoreMessageID {
			messageIDs = append(messageIDs, msg.ID)
		}
	}

	if len(messageIDs) == 0 {
		return Result{Name: "ClearChannelEmpty", Count: 0}
	}

	stdlog.Printf("[DISCORD_BULK_DELETE] ChannelID: %s | Count: %d | Cleanup operation\n", channelID, len(messageIDs))
	err = s.ChannelMessagesBulkDelete(channelID, messageIDs)
	if err != nil {
		log.Error("Failed to bulk delete messages, falling back to individual deletion", err)
		// Fallback for older messages
		for _, id := range messageIDs {
			stdlog.Printf("[DISCORD_DELETE] ChannelID: %s | MessageID: %s | Cleanup fallback\n", channelID, id)
			if err := s.ChannelMessageDelete(channelID, id); err != nil {
				log.Error("Failed to delete message", err)
			}
		}
	}

	var resultName string
	if channelID == discordCfg.LogChannelID {
		resultName = "ClearLogs"
	} else {
		resultName = "ClearTranscriptions"
	}
	return Result{Name: resultName, Count: len(messageIDs)}
}

// CleanStaleMessages updates any of the bot's messages that were left in a pending state.
func CleanStaleMessages(s *discordgo.Session, channelID string, log logger.Logger) Result {
	if channelID == "" {
		return Result{Name: "CleanStaleSkipped", Count: 0}
	}
	messages, err := s.ChannelMessages(channelID, 100, "", "", "")
	if err != nil {
		log.Error("Failed to get messages for stale message cleanup", err)
		return Result{Name: "CleanStaleFailed", Count: 0}
	}

	count := 0
	for _, msg := range messages {
		if msg.Author.ID == s.State.User.ID {
			if strings.Contains(msg.Content, "[speaking...]") || strings.Contains(msg.Content, "[awaiting transcription]") {
				newContent := strings.Split(msg.Content, "|")[0] + "| `Status: Interrupted (bot restarted)`"
				stdlog.Printf("[DISCORD_EDIT] MessageID: %s | Stale message cleanup\n", msg.ID)
				if _, err := s.ChannelMessageEdit(channelID, msg.ID, newContent); err != nil {
					log.Error("Failed to edit stale message", err)
				}
				count++
			}
		}
	}
	return Result{Name: "CleanStaleMessages", Count: count}
}
