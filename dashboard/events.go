package dashboard

import (
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

// EventsDashboard shows recent Discord events
type EventsDashboard struct {
	session      *discordgo.Session
	logChannelID string
	cache        *MessageCache
}

// NewEventsDashboard creates a new events dashboard
func NewEventsDashboard(session *discordgo.Session, logChannelID string) *EventsDashboard {
	return &EventsDashboard{
		session:      session,
		logChannelID: logChannelID,
		cache: &MessageCache{
			ThrottleDuration: 30 * time.Second,
		},
	}
}

// Init creates the events dashboard message
func (d *EventsDashboard) Init() error {
	content := "**Events Dashboard**\n\n_No events yet_"

	log.Println("[DASHBOARD_INIT] Creating Events dashboard...")

	msg, err := d.session.ChannelMessageSend(d.logChannelID, content)
	if err != nil {
		return fmt.Errorf("failed to create events dashboard: %w", err)
	}

	d.cache.MessageID = msg.ID
	d.cache.Content = content
	d.cache.LastUpdate = time.Now()
	d.cache.LastAPIUpdate = time.Now()

	log.Printf("[DASHBOARD_INIT] Events dashboard created: %s\n", msg.ID)

	return nil
}

// Update refreshes the events dashboard (throttled)
func (d *EventsDashboard) Update() error {
	// TODO: Implement actual event tracking
	return nil
}

// ForceUpdate bypasses throttle
func (d *EventsDashboard) ForceUpdate() error {
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, d.cache.Content)
}

// Finalize performs final update
func (d *EventsDashboard) Finalize() error {
	return nil
}
