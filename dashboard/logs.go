package dashboard

import (
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

// LogsDashboard shows recent logs and errors
type LogsDashboard struct {
	session      *discordgo.Session
	logChannelID string
	cache        *MessageCache
}

// NewLogsDashboard creates a new logs dashboard
func NewLogsDashboard(session *discordgo.Session, logChannelID string) *LogsDashboard {
	return &LogsDashboard{
		session:      session,
		logChannelID: logChannelID,
		cache: &MessageCache{
			ThrottleDuration: 30 * time.Second,
		},
	}
}

// Init creates the logs dashboard message
func (d *LogsDashboard) Init() error {
	content := "**Logs Dashboard**\n\n_No logs yet_"

	log.Println("[DASHBOARD_INIT] Creating Logs dashboard...")

	msg, err := d.session.ChannelMessageSend(d.logChannelID, content)
	if err != nil {
		return fmt.Errorf("failed to create logs dashboard: %w", err)
	}

	d.cache.MessageID = msg.ID
	d.cache.Content = content
	d.cache.LastUpdate = time.Now()
	d.cache.LastAPIUpdate = time.Now()

	log.Printf("[DASHBOARD_INIT] Logs dashboard created: %s\n", msg.ID)

	return nil
}

// Update refreshes the logs dashboard (throttled)
func (d *LogsDashboard) Update() error {
	// TODO: Implement actual log tracking
	return nil
}

// ForceUpdate bypasses throttle
func (d *LogsDashboard) ForceUpdate() error {
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, d.cache.Content)
}

// Finalize performs final update
func (d *LogsDashboard) Finalize() error {
	return nil
}
