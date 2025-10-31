package dashboard

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

const maxMessages = 100

// MessagesDashboard shows recent Discord messages
type MessagesDashboard struct {
	session        *discordgo.Session
	logChannelID   string
	cache          *MessageCache
	recentMessages []string // In-memory store for recent messages
}

// NewMessagesDashboard creates a new messages dashboard
func NewMessagesDashboard(session *discordgo.Session, logChannelID string) *MessagesDashboard {
	return &MessagesDashboard{
		session:      session,
		logChannelID: logChannelID,
		cache: &MessageCache{
			ThrottleDuration: 30 * time.Second,
		},
		recentMessages: make([]string, 0, maxMessages),
	}
}

// Init creates the messages dashboard message
func (d *MessagesDashboard) Init() error {
	content := "**Messages Dashboard**\n\n_No messages yet_"

	log.Println("[DASHBOARD_INIT] Creating Messages dashboard...")

	msg, err := d.session.ChannelMessageSend(d.logChannelID, content)
	if err != nil {
		return fmt.Errorf("failed to create messages dashboard: %w", err)
	}

	d.cache.MessageID = msg.ID
	d.cache.Content = content
	d.cache.LastUpdate = time.Now()
	d.cache.LastAPIUpdate = time.Now()

	log.Printf("[DASHBOARD_INIT] Messages dashboard created: %s\n", msg.ID)

	return nil
}

// AddMessage adds a new message to the dashboard's log and triggers an update.
func (d *MessagesDashboard) AddMessage(message string) {
	// TODO: Replace in-memory slice with Redis list (LPUSH/LTRIM)
	d.recentMessages = append(d.recentMessages, message)
	if len(d.recentMessages) > maxMessages {
		// Trim the slice to keep only the last `maxMessages` messages.
		d.recentMessages = d.recentMessages[len(d.recentMessages)-maxMessages:]
	}

	// Trigger a throttled update.
	if err := d.Update(); err != nil {
		log.Printf("Error updating messages dashboard: %v", err)
	}
}

// Update refreshes the messages dashboard (throttled)
func (d *MessagesDashboard) Update() error {
	content := d.formatMessages()
	return UpdateThrottled(d.cache, d.session, d.logChannelID, content)
}

// ForceUpdate bypasses throttle
func (d *MessagesDashboard) ForceUpdate() error {
	content := d.formatMessages()
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, content)
}

// Finalize performs final update
func (d *MessagesDashboard) Finalize() error {
	return nil
}

// formatMessages generates the display content for the dashboard.
func (d *MessagesDashboard) formatMessages() string {
	if len(d.recentMessages) == 0 {
		return "**Messages Dashboard**\n\n_No messages yet_"
	}

	var builder strings.Builder
	builder.WriteString("**Messages Dashboard**\n```\n")

	// Get the last 10 messages
	start := 0
	if len(d.recentMessages) > 10 {
		start = len(d.recentMessages) - 10
	}

	for _, msg := range d.recentMessages[start:] {
		builder.WriteString(msg)
		builder.WriteString("\n")
	}

	builder.WriteString("```")
	return builder.String()
}
