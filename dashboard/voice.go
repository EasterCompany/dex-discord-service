package dashboard

import (
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

// VoiceDashboard shows voice connection status
type VoiceDashboard struct {
	session      *discordgo.Session
	logChannelID string
	cache        *MessageCache
}

// NewVoiceDashboard creates a new voice dashboard
func NewVoiceDashboard(session *discordgo.Session, logChannelID string) *VoiceDashboard {
	return &VoiceDashboard{
		session:      session,
		logChannelID: logChannelID,
		cache: &MessageCache{
			ThrottleDuration: 30 * time.Second,
		},
	}
}

// Init creates the voice dashboard message
func (d *VoiceDashboard) Init() error {
	content := "**Voice Dashboard**\n\n❌ **Status:** Not connected"

	log.Println("[DASHBOARD_INIT] Creating Voice dashboard...")

	msg, err := d.session.ChannelMessageSend(d.logChannelID, content)
	if err != nil {
		return fmt.Errorf("failed to create voice dashboard: %w", err)
	}

	d.cache.MessageID = msg.ID
	d.cache.Content = content
	d.cache.LastUpdate = time.Now()
	d.cache.LastAPIUpdate = time.Now()

	log.Printf("[DASHBOARD_INIT] Voice dashboard created: %s\n", msg.ID)

	return nil
}

// Update refreshes the voice dashboard (throttled)
func (d *VoiceDashboard) Update() error {
	// TODO: Implement actual voice status tracking
	return nil
}

// ForceUpdate bypasses throttle
func (d *VoiceDashboard) ForceUpdate() error {
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, d.cache.Content)
}

// Finalize performs final update
func (d *VoiceDashboard) Finalize() error {
	return nil
}
