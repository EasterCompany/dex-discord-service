package cleanup

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

type Result struct {
	Name  string
	Count int
}

// ClearChannel removes all messages from a channel, except for a specified message to ignore.
func ClearChannel(s *discordgo.Session, channelID, ignoreMessageID string, discordCfg *config.DiscordConfig) Result {
	if channelID == "" {
		return Result{Name: "ClearChannelSkipped", Count: 0}
	}
	messages, err := s.ChannelMessages(channelID, 100, "", "", "")
	if err != nil {
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

	err = s.ChannelMessagesBulkDelete(channelID, messageIDs)
	if err != nil {
		// Fallback for older messages
		for _, id := range messageIDs {
			s.ChannelMessageDelete(channelID, id)
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
func CleanStaleMessages(s *discordgo.Session, channelID string) Result {
	if channelID == "" {
		return Result{Name: "CleanStaleSkipped", Count: 0}
	}
	messages, err := s.ChannelMessages(channelID, 100, "", "", "")
	if err != nil {
		return Result{Name: "CleanStaleFailed", Count: 0}
	}

	count := 0
	for _, msg := range messages {
		if msg.Author.ID == s.State.User.ID {
			if strings.Contains(msg.Content, "[speaking...]") || strings.Contains(msg.Content, "[awaiting transcription]") {
				newContent := strings.Split(msg.Content, "|")[0] + "| `Status: Interrupted (bot restarted)`"
				s.ChannelMessageEdit(channelID, msg.ID, newContent)
				count++
			}
		}
	}
	return Result{Name: "CleanStaleMessages", Count: count}
}
