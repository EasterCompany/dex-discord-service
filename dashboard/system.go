package dashboard

import (
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

// SystemDashboard shows system health and boot status
type SystemDashboard struct {
	session      *discordgo.Session
	logChannelID string
	cache        *MessageCache
}

// NewSystemDashboard creates a new system dashboard
func NewSystemDashboard(session *discordgo.Session, logChannelID string) *SystemDashboard {
	return &SystemDashboard{
		session:      session,
		logChannelID: logChannelID,
		cache: &MessageCache{
			ThrottleDuration: 60 * time.Second, // Update at most once per 60 seconds
		},
	}
}

// Init creates the system dashboard message
func (d *SystemDashboard) Init() error {
	content := d.formatBootMessage()

	log.Println("[DASHBOARD_INIT] Creating System dashboard...")

	msg, err := d.session.ChannelMessageSend(d.logChannelID, content)
	if err != nil {
		return fmt.Errorf("failed to create system dashboard: %w", err)
	}

	d.cache.MessageID = msg.ID
	d.cache.Content = content
	d.cache.LastUpdate = time.Now()
	d.cache.LastAPIUpdate = time.Now()

	log.Printf("[DASHBOARD_INIT] System dashboard created: %s\n", msg.ID)

	return nil
}

// Update refreshes the system dashboard (throttled)
func (d *SystemDashboard) Update() error {
	content := d.formatHealthMessage()
	return UpdateThrottled(d.cache, d.session, d.logChannelID, content)
}

// ForceUpdate bypasses throttle
func (d *SystemDashboard) ForceUpdate() error {
	content := d.formatHealthMessage()
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, content)
}

// Finalize performs final update
func (d *SystemDashboard) Finalize() error {
	content := "**System Dashboard**\n\n⏹️ **Status:** Offline\n\n_Bot shutting down_"
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, content)
}

// formatBootMessage creates the initial boot message
func (d *SystemDashboard) formatBootMessage() string {
	return "**System Dashboard**\n\n🔄 **Status:** Starting up...\n\n_Initializing services_"
}

// formatHealthMessage creates the health status message
func (d *SystemDashboard) formatHealthMessage() string {
	// TODO: Gather actual system metrics
	return fmt.Sprintf("**System Dashboard**\n\n"+
		"✅ **Status:** Running\n"+
		"🕒 **Uptime:** %s\n\n"+
		"**Services:**\n"+
		"✅ Discord\n"+
		"✅ Redis\n"+
		"\n_Last updated: %s_",
		"calculating...",
		time.Now().Format("15:04:05"))
}
