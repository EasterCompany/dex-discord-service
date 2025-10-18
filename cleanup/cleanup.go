package cleanup

import (
	"fmt"
	"strings"
	"time"

	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/bwmarrin/discordgo"
)

// Result holds the outcome of a cleanup task.
type Result struct {
	Name        string
	Count       int
	Description string
}

// ClearChannel fetches and deletes up to 100 messages from a channel.
func ClearChannel(s *discordgo.Session, channelID string, ignoreMessageID string) Result {
	res := Result{Name: "ClearChannel", Description: fmt.Sprintf("ch: %s", channelID)}
	if channelID == "" {
		return res
	}

	messages, err := s.ChannelMessages(channelID, 100, "", "", "")
	if err != nil {
		logger.Error(fmt.Sprintf("Could not fetch messages from channel %s to clear them", channelID), err)
		return res
	}

	var messageIDs []string
	for _, msg := range messages {
		if ignoreMessageID != "" && msg.ID == ignoreMessageID {
			continue
		}
		// Discord API does not allow bulk deleting messages older than 2 weeks
		if time.Since(msg.Timestamp) > 14*24*time.Hour {
			continue
		}
		messageIDs = append(messageIDs, msg.ID)
	}

	if len(messageIDs) == 0 {
		return res
	}

	if err := s.ChannelMessagesBulkDelete(channelID, messageIDs); err != nil {
		logger.Error(fmt.Sprintf("Could not bulk delete messages from channel %s", channelID), err)
	} else {
		res.Count = len(messageIDs)
	}

	return res
}

// CleanStaleMessages finds and updates messages from a previous session.
func CleanStaleMessages(s *discordgo.Session, channelID string) Result {
	res := Result{Name: "CleanStaleMessages"}
	if channelID == "" {
		return res
	}
	messages, err := s.ChannelMessages(channelID, 100, "", "", "")
	if err != nil {
		logger.Error("Could not fetch messages to clean stale ones", err)
		return res
	}

	for _, msg := range messages {
		if msg.Author.ID == s.State.User.ID {
			if strings.Contains(msg.Content, "[speaking...]") || strings.Contains(msg.Content, "[awaiting transcription]") {
				newContent := fmt.Sprintf("`%s` **%s**: ⚫ [interrupted by restart]", time.Now().Format("15:04:05"), msg.Author.Username)
				_, err := s.ChannelMessageEdit(channelID, msg.ID, newContent)
				if err == nil {
					res.Count++
				}
				time.Sleep(300 * time.Millisecond) // Be respectful of rate limits
			}
		}
	}
	return res
}
