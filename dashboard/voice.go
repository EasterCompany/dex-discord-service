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
	session       *discordgo.Session
	logChannelID  string
	cache         *MessageCache
	voiceState    *VoiceState
	speakingUsers map[string]bool // UserID -> isSpeaking

	// Audio statistics
	audioStats *AudioStatistics
}

// AudioStatistics tracks audio-related statistics
type AudioStatistics struct {
	TotalPackets   int64
	TotalBytes     int64
	ActiveSpeakers int
	LastPacket     time.Time
	SessionStart   time.Time
	BotConnected   bool
	BotChannelName string
}

// NewVoiceDashboard creates a new voice dashboard
func NewVoiceDashboard(session *discordgo.Session, logChannelID string, voiceState *VoiceState) *VoiceDashboard {
	return &VoiceDashboard{
		session:      session,
		logChannelID: logChannelID,
		cache: &MessageCache{
			ThrottleDuration: 5 * time.Second, // Faster updates for voice state
		},
		voiceState:    voiceState,
		speakingUsers: make(map[string]bool),
		audioStats: &AudioStatistics{
			SessionStart: time.Now(),
		},
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

// SetUserSpeaking updates the speaking status of a user
func (d *VoiceDashboard) SetUserSpeaking(userID string, isSpeaking bool) {
	d.speakingUsers[userID] = isSpeaking
}

// UpdateAudioStats updates the audio statistics
func (d *VoiceDashboard) UpdateAudioStats(packets, bytes int64, activeSpeakers int, lastPacket time.Time) {
	d.audioStats.TotalPackets = packets
	d.audioStats.TotalBytes = bytes
	d.audioStats.ActiveSpeakers = activeSpeakers
	d.audioStats.LastPacket = lastPacket
}

// SetBotConnection updates the bot's voice connection status
func (d *VoiceDashboard) SetBotConnection(connected bool, channelName string) {
	d.audioStats.BotConnected = connected
	d.audioStats.BotChannelName = channelName
	if connected {
		d.audioStats.SessionStart = time.Now()
	}
}

// formatVoiceState generates the display content for the dashboard.
func (d *VoiceDashboard) formatVoiceState() string {
	channels := d.voiceState.GetChannels()

	var builder strings.Builder
	builder.WriteString("**Voice Dashboard**\n\n")

	// Bot connection status
	if d.audioStats.BotConnected {
		builder.WriteString(fmt.Sprintf("🤖 **Bot Connected:** #%s\n", d.audioStats.BotChannelName))
		sessionDuration := time.Since(d.audioStats.SessionStart).Round(time.Second)
		builder.WriteString(fmt.Sprintf("⏱️ **Session Duration:** %s\n", sessionDuration))
		builder.WriteString("\n")

		// Audio statistics
		if d.audioStats.TotalPackets > 0 {
			builder.WriteString("**Audio Statistics:**\n")
			builder.WriteString(fmt.Sprintf(" - Total Packets: %d\n", d.audioStats.TotalPackets))
			builder.WriteString(fmt.Sprintf(" - Total Bytes: %d (%.2f MB)\n", d.audioStats.TotalBytes, float64(d.audioStats.TotalBytes)/(1024*1024)))
			if !d.audioStats.LastPacket.IsZero() {
				timeSince := time.Since(d.audioStats.LastPacket).Round(time.Millisecond)
				builder.WriteString(fmt.Sprintf(" - Last Packet: %s ago\n", timeSince))
			}
			builder.WriteString(fmt.Sprintf(" - Active Speakers: %d\n", d.audioStats.ActiveSpeakers))
			builder.WriteString("\n")
		}
	}

	// Voice channel status
	if len(channels) == 0 {
		if !d.audioStats.BotConnected {
			builder.WriteString("❌ **Status:** No active voice channels")
		}
	} else {
		builder.WriteString("**Active Voice Channels:**\n\n")
		for channelID, users := range channels {
			channel, err := d.session.State.Channel(channelID)
			channelName := channelID
			if err == nil {
				channelName = channel.Name
			}

			builder.WriteString(fmt.Sprintf("🔊 **%s** (%d users)\n", channelName, len(users)))
			for userID, username := range users {
				indicator := "   "
				if d.speakingUsers[userID] {
					indicator = "🎙️"
				}
				builder.WriteString(fmt.Sprintf(" %s %s\n", indicator, username))
			}
			builder.WriteString("\n")
		}
	}

	return builder.String()
}
