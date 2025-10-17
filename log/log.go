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
	log.Println("Initializing logger...")
	session = s
	logChannelID = channelID
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Println("Discord session is ready.")
		close(ready)
	})
	log.SetOutput(&discordWriter{})
}

// Post sends a message to the log channel
func Post(msg string) {
	if session != nil && logChannelID != "" {
		session.ChannelMessageSend(logChannelID, msg)
	}
}

// PostInitialMessage sends an initial message and returns the message object
func PostInitialMessage(msg string) (*discordgo.Message, error) {
	log.Println("Waiting for Discord session to be ready...")
	<-ready
	log.Println("Discord session is ready. Posting initial message...")
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

// discordWriter is a writer that sends messages to the discord channel
type discordWriter struct{}

func (w *discordWriter) Write(p []byte) (n int, err error) {
	Post(string(p))
	return len(p), nil
}
