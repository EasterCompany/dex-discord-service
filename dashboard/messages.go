package dashboard

import (
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

// MessagesDashboard shows recent Discord messages
type MessagesDashboard struct {
	session      *discordgo.Session
	logChannelID string
	cache        *MessageCache
}

// NewMessagesDashboard creates a new messages dashboard
func NewMessagesDashboard(session *discordgo.Session, logChannelID string) *MessagesDashboard {
	return &MessagesDashboard{
		session:      session,
		logChannelID: logChannelID,
		cache: &MessageCache{
			ThrottleDuration: 30 * time.Second,
		},
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

// Update refreshes the messages dashboard (throttled)
func (d *MessagesDashboard) Update() error {
	// TODO: Implement actual message tracking
	return nil
}

// ForceUpdate bypasses throttle
func (d *MessagesDashboard) ForceUpdate() error {
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, d.cache.Content)
}

// Finalize performs final update
func (d *MessagesDashboard) Finalize() error {
	return nil
}
