package log

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

var (
	session      *discordgo.Session
	logChannelID string
	ready        = make(chan struct{})
)

// Init initializes the log module with a discord session
func Init(s *discordgo.Session, channelID string) {
	session = s
	logChannelID = channelID
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		close(ready)
	})
}

// Post sends a message to the log channel
func Post(msg string) {
	if session != nil && logChannelID != "" {
		// Ensure the message is not too long for Discord
		if len(msg) > 2000 {
			msg = msg[:1997] + "..."
		}
		session.ChannelMessageSend(logChannelID, msg)
	}
}

// Error logs an error to the console and posts it to the Discord log channel.
func Error(context string, err error) {
	errorMessage := fmt.Sprintf("❌ **Error** | Context: `%s`\n```\n%v\n```", context, err)

	// Log to console
	log.Println(errorMessage)

	// Post to Discord
	Post(errorMessage)
}

// PostInitialMessage sends an initial message and returns the message object
func PostInitialMessage(msg string) (*discordgo.Message, error) {
	<-ready
	if session != nil && logChannelID != "" {
		return session.ChannelMessageSend(logChannelID, msg)
	}
	return nil, fmt.Errorf("session not initialized")
}

// UpdateInitialMessage edits the initial message with new content
func UpdateInitialMessage(messageID, newContent string) {
	if session != nil && logChannelID != "" {
		session.ChannelMessageEdit(logChannelID, messageID, newContent)
	}
}
