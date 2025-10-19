package log

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type Logger interface {
	Post(msg string)
	PostInitialMessage(msg string) (*discordgo.Message, error)
	UpdateInitialMessage(messageID, newContent string)
	Error(context string, err error)
	Fatal(context string, err error)
}

type logger struct {
	session      *discordgo.Session
	logChannelID string
}

// NewLogger creates a new logger
func NewLogger(s *discordgo.Session, channelID string) Logger {
	return &logger{
		session:      s,
		logChannelID: channelID,
	}
}

// Post sends a message to the log channel
func (l *logger) Post(msg string) {
	if l.session != nil && l.logChannelID != "" {
		_, _ = l.session.ChannelMessageSend(l.logChannelID, msg)
	}
}

// PostInitialMessage sends an initial message and returns the message object
func (l *logger) PostInitialMessage(msg string) (*discordgo.Message, error) {
	if l.session != nil && l.logChannelID != "" {
		return l.session.ChannelMessageSend(l.logChannelID, msg)
	}
	return nil, fmt.Errorf("session not initialized")
}

// UpdateInitialMessage edits the initial message with new content
func (l *logger) UpdateInitialMessage(messageID, newContent string) {
	if l.session != nil && l.logChannelID != "" {
		_, _ = l.session.ChannelMessageEdit(l.logChannelID, messageID, newContent)
	}
}

// Error logs an error to the console and to the discord channel
func (l *logger) Error(context string, err error) {
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
	if l.session != nil && l.logChannelID != "" {
		msg := fmt.Sprintf("```\n[ERROR] in %s: %s\n%v\n```", callerInfo, context, err)
		if len(msg) > 1900 {
			msg = msg[:1900] + "..."
		}
		_, _ = l.session.ChannelMessageSend(l.logChannelID, msg)
	}
}

// Fatal logs an error and then exits the program.
func (l *logger) Fatal(context string, err error) {
	l.Error(context, err)
	os.Exit(1)
}
