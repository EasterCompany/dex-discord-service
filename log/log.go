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
	ready        = make(chan struct{})
)

// Init initializes the log module with a discord session
func Init(s *discordgo.Session, channelID string) {
	session = s
	logChannelID = channelID
	// Use a handler to know when the session is ready.
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		close(ready)
	})
	log.SetOutput(&discordWriter{})
	log.SetFlags(0) // We will handle timestamping and file info ourselves.
}

// Post sends a message to the log channel
func Post(msg string) {
	if session != nil && logChannelID != "" {
		// Wait until the session is ready before trying to send a message.
		<-ready
		session.ChannelMessageSend(logChannelID, msg)
	}
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

	// Log to standard logger (which includes our discordWriter)
	log.Printf("[ERROR] in %s: %s\n%v\n", callerInfo, context, err)
}

// Fatal logs an error and then exits the program.
func Fatal(context string, err error) {
	Error(context, err)
	os.Exit(1)
}

// discordWriter is a writer that sends messages to the discord channel
type discordWriter struct{}

func (w *discordWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	// Log to console as well
	fmt.Print(msg)
	// Send to Discord
	if session != nil && logChannelID != "" {
		// To prevent log spam, we truncate long messages for Discord.
		if len(msg) > 1900 {
			msg = msg[:1900] + "..."
		}
		Post("```\n" + msg + "```")
	}
	return len(p), nil
}
