package dashboard

import (
	"time"

	"github.com/bwmarrin/discordgo"
)

// Dashboard represents a persistent message panel in Discord
type Dashboard interface {
	// Init creates the dashboard message in Discord
	Init() error

	// Update refreshes the dashboard content (throttled)
	Update() error

	// ForceUpdate bypasses throttle and updates immediately
	ForceUpdate() error

	// Finalize performs final update and cleanup
	Finalize() error
}

// MessageCache holds the cached state of a dashboard
type MessageCache struct {
	MessageID       string
	Content         string
	LastUpdate      time.Time
	LastAPIUpdate   time.Time
	ThrottleDuration time.Duration
}

// Session is a minimal interface for Discord operations
type Session interface {
	ChannelMessageSend(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessageEdit(channelID, messageID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
}
