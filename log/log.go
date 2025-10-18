package log

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var (
	session      *discordgo.Session
	logChannelID string
)

// Init initializes the log module with a discord session
func Init(s *discordgo.Session, channelID string) {
	session = s
	logChannelID = channelID
}

// Post sends a message to the log channel
func Post(msg string) {
	if session != nil && logChannelID != "" {
		session.ChannelMessageSend(logChannelID, msg)
	}
}

// PostInitialMessage sends an initial message and returns the message object
func PostInitialMessage(msg string) (*discordgo.Message, error) {
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

// Error logs an error to the console and to the discord channel
func Error(context string, err error) {
	// Get caller info
	_, file, line, ok := runtime.Caller(1)
	var callerInfo string
	if ok {
		parts := strings.Split(file, "/")
		if len(parts) > 2 {
			file = strings.Join(parts[len(parts)-2:], "/")
		}
		callerInfo = fmt.Sprintf("%s:%d", file, line)
	}

	// Log to console
	log.Printf("[ERROR] in %s: %s\n%v\n", callerInfo, context, err)

	// Send to Discord
	if session != nil && logChannelID != "" {
		msg := fmt.Sprintf("```\n[ERROR] in %s: %s\n%v\n```", callerInfo, context, err)
		if len(msg) > 1900 {
			msg = msg[:1900] + "..."
		}
		session.ChannelMessageSend(logChannelID, msg)
	}
}

// Fatal logs an error and then exits the program.
func Fatal(context string, err error) {
	Error(context, err)
	os.Exit(1)
}
