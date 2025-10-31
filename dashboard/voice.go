package dashboard

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// VoiceDashboard shows voice connection status
type VoiceDashboard struct {
	session      *discordgo.Session
	logChannelID string
	cache        *MessageCache
	voiceState   *VoiceState
}

// NewVoiceDashboard creates a new voice dashboard
func NewVoiceDashboard(session *discordgo.Session, logChannelID string, voiceState *VoiceState) *VoiceDashboard {
	return &VoiceDashboard{
		session:      session,
		logChannelID: logChannelID,
		cache: &MessageCache{
			ThrottleDuration: 5 * time.Second, // Faster updates for voice state
		},
		voiceState: voiceState,
	}
}

// Init creates the voice dashboard message
func (d *VoiceDashboard) Init() error {
	log.Println("[DASHBOARD_INIT] Creating Voice dashboard...")

	content := d.formatVoiceState()
	msg, err := d.session.ChannelMessageSend(d.logChannelID, content)
	if err != nil {
		return fmt.Errorf("failed to create voice dashboard: %w", err)
	}

	d.cache.MessageID = msg.ID
	d.cache.Content = content
	d.cache.LastUpdate = time.Now()
	d.cache.LastAPIUpdate = time.Now()

	log.Printf("[DASHBOARD_INIT] Voice dashboard created: %s\n", msg.ID)

	return d.Update() // Perform initial update
}

// Update refreshes the voice dashboard (throttled)
func (d *VoiceDashboard) Update() error {
	content := d.formatVoiceState()
	return UpdateThrottled(d.cache, d.session, d.logChannelID, content)
}

// ForceUpdate bypasses throttle
func (d *VoiceDashboard) ForceUpdate() error {
	content := d.formatVoiceState()
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, content)
}

// Finalize performs final update
func (d *VoiceDashboard) Finalize() error {
	content := "**Voice Dashboard**\n\n⏹️ **Status:** Offline"
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, content)
}

// formatVoiceState generates the display content for the dashboard.
func (d *VoiceDashboard) formatVoiceState() string {
	channels := d.voiceState.GetChannels()

	if len(channels) == 0 {
		return "**Voice Dashboard**\n\n❌ **Status:** No active voice channels"
	}

	var builder strings.Builder
	builder.WriteString("**Voice Dashboard**\n\n")

	for channelID, users := range channels {
		channel, err := d.session.State.Channel(channelID)
		channelName := channelID
		if err == nil {
			channelName = channel.Name
		}

		builder.WriteString(fmt.Sprintf("🔊 **%s**\n", channelName))
		for _, username := range users {
			builder.WriteString(fmt.Sprintf(" - @%s\n", username))
		}
		builder.WriteString("\n")
	}

	return builder.String()
}
