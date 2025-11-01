package context

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/EasterCompany/dex-discord-service/cache"
	"github.com/EasterCompany/dex-discord-service/config"
	"github.com/EasterCompany/dex-discord-service/dashboard"
	"github.com/bwmarrin/discordgo"
)

// Snapshot represents a point-in-time capture of the Discord interface state
// This is the context that gets sent to the LLM for generating responses
type Snapshot struct {
	Timestamp time.Time `json:"timestamp"`

	// Server context
	ServerName  string `json:"server_name"`
	ServerID    string `json:"server_id"`
	OwnerID     string `json:"owner_id"`
	MemberCount int    `json:"member_count"`

	// Server persona
	Persona config.ServerPersona `json:"persona"`

	// Recent activity
	RecentLogs     []string `json:"recent_logs"`
	RecentEvents   []string `json:"recent_events"`
	RecentMessages []string `json:"recent_messages"`

	// Voice state
	VoiceChannels   map[string]map[string]string `json:"voice_channels"` // channelID -> userID -> username
	BotInVoice      bool                         `json:"bot_in_voice"`
	BotVoiceChannel string                       `json:"bot_voice_channel,omitempty"`

	// Bot state
	BotStatus string `json:"bot_status"` // Sleeping, Idle, Thinking
	Uptime    string `json:"uptime"`

	// Trigger context
	TriggerUser    string `json:"trigger_user"`
	TriggerChannel string `json:"trigger_channel"`
	TriggerMessage string `json:"trigger_message"`
}

// SnapshotBuilder creates context snapshots from dashboard state
type SnapshotBuilder struct {
	session      *discordgo.Session
	dashboardMgr *dashboard.Manager
	redisClient  *cache.RedisClient
	config       *config.Config
	startTime    time.Time
}

// NewSnapshotBuilder creates a new context snapshot builder
func NewSnapshotBuilder(
	session *discordgo.Session,
	dashboardMgr *dashboard.Manager,
	redisClient *cache.RedisClient,
	cfg *config.Config,
	startTime time.Time,
) *SnapshotBuilder {
	return &SnapshotBuilder{
		session:      session,
		dashboardMgr: dashboardMgr,
		redisClient:  redisClient,
		config:       cfg,
		startTime:    startTime,
	}
}

// CaptureSnapshot creates a snapshot of the current state
// This is called when the bot needs to respond to a user
func (sb *SnapshotBuilder) CaptureSnapshot(
	triggerUser, triggerChannel, triggerMessage, botStatus string,
) (*Snapshot, error) {
	ctx := context.Background()
	snapshot := &Snapshot{
		Timestamp:      time.Now(),
		BotStatus:      botStatus,
		Uptime:         time.Since(sb.startTime).Round(time.Second).String(),
		TriggerUser:    triggerUser,
		TriggerChannel: triggerChannel,
		TriggerMessage: triggerMessage,
	}

	// Capture server info
	if guild, err := sb.session.Guild(sb.dashboardMgr.Server.GetServerID()); err == nil {
		snapshot.ServerName = guild.Name
		snapshot.ServerID = guild.ID
		snapshot.OwnerID = guild.OwnerID
		snapshot.MemberCount = guild.MemberCount
	}

	// Capture server persona configuration
	snapshot.Persona = sb.config.ServerPersona

	// Capture recent logs (last 10 from Redis)
	if logs, err := sb.redisClient.GetListRange(ctx, cache.LogsKey, 0, 9); err == nil {
		snapshot.RecentLogs = logs
	}

	// Capture recent events (last 15 from Redis)
	if events, err := sb.redisClient.GetListRange(ctx, cache.EventsKey, 0, 14); err == nil {
		snapshot.RecentEvents = events
	}

	// Capture recent messages (last 10 from Redis)
	if messages, err := sb.redisClient.GetListRange(ctx, cache.MessagesKey, 0, 9); err == nil {
		snapshot.RecentMessages = messages
	}

	// Capture voice state
	voiceChannels := sb.dashboardMgr.VoiceState.GetChannels()
	snapshot.VoiceChannels = voiceChannels

	// Check if bot is in voice (simplified - can be enhanced)
	snapshot.BotInVoice = false // TODO: Track this in VoiceConnectionManager

	return snapshot, nil
}

// FormatAsJSON converts the snapshot to JSON for structured LLM consumption
// Use this if the LLM service prefers JSON format
// For human-readable format, use FormatForLLM()

// FormatForLLM converts the snapshot into a human-readable context string
// This is what gets sent to the LLM as context
func (s *Snapshot) FormatForLLM() string {
	var builder strings.Builder

	builder.WriteString("=== DEXTER CONTEXT SNAPSHOT ===\n\n")
	builder.WriteString(fmt.Sprintf("Time: %s\n", s.Timestamp.Format("2006-01-02 15:04:05")))
	builder.WriteString(fmt.Sprintf("Bot Uptime: %s\n", s.Uptime))
	builder.WriteString(fmt.Sprintf("Bot Status: %s\n\n", s.BotStatus))

	// Server context
	builder.WriteString("--- SERVER INFO ---\n")
	builder.WriteString(fmt.Sprintf("Server: %s (ID: %s)\n", s.ServerName, s.ServerID))
	builder.WriteString(fmt.Sprintf("Members: %d\n", s.MemberCount))
	builder.WriteString(fmt.Sprintf("Owner: <@%s>\n\n", s.OwnerID))

	// Persona context
	if s.Persona.Enabled {
		builder.WriteString("--- BOT PERSONA ---\n")
		if s.Persona.Name != "" {
			builder.WriteString(fmt.Sprintf("Name: %s\n", s.Persona.Name))
		}
		if s.Persona.Personality != "" {
			builder.WriteString(fmt.Sprintf("Personality: %s\n", s.Persona.Personality))
		}
		if s.Persona.ResponseStyle != "" {
			builder.WriteString(fmt.Sprintf("Response Style: %s\n", s.Persona.ResponseStyle))
		}
		if len(s.Persona.BehaviorRules) > 0 {
			builder.WriteString("Behavior Rules:\n")
			for _, rule := range s.Persona.BehaviorRules {
				builder.WriteString(fmt.Sprintf("  - %s\n", rule))
			}
		}
		if s.Persona.SystemPrompt != "" {
			builder.WriteString(fmt.Sprintf("\nSystem Prompt:\n%s\n", s.Persona.SystemPrompt))
		}
		builder.WriteString("\n")
	}

	// Trigger context
	builder.WriteString("--- CURRENT INTERACTION ---\n")
	builder.WriteString(fmt.Sprintf("User: %s\n", s.TriggerUser))
	builder.WriteString(fmt.Sprintf("Channel: %s\n", s.TriggerChannel))
	builder.WriteString(fmt.Sprintf("Message: \"%s\"\n\n", s.TriggerMessage))

	// Voice state
	if len(s.VoiceChannels) > 0 {
		builder.WriteString("--- VOICE CHANNELS ---\n")
		for channelID, users := range s.VoiceChannels {
			builder.WriteString(fmt.Sprintf("Channel %s: %d users\n", channelID, len(users)))
			for _, username := range users {
				builder.WriteString(fmt.Sprintf("  - %s\n", username))
			}
		}
		builder.WriteString("\n")
	}

	if s.BotInVoice {
		builder.WriteString(fmt.Sprintf("Bot is connected to voice channel: %s\n\n", s.BotVoiceChannel))
	}

	// Recent events
	if len(s.RecentEvents) > 0 {
		builder.WriteString("--- RECENT EVENTS ---\n")
		// Reverse to show newest last (chronological)
		for i := len(s.RecentEvents) - 1; i >= 0; i-- {
			builder.WriteString(s.RecentEvents[i])
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	// Recent messages
	if len(s.RecentMessages) > 0 {
		builder.WriteString("--- RECENT MESSAGES ---\n")
		// Reverse to show newest last (chronological)
		for i := len(s.RecentMessages) - 1; i >= 0; i-- {
			builder.WriteString(s.RecentMessages[i])
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	// Recent logs (system state)
	if len(s.RecentLogs) > 0 {
		builder.WriteString("--- RECENT SYSTEM LOGS ---\n")
		// Reverse to show newest last (chronological)
		for i := len(s.RecentLogs) - 1; i >= 0; i-- {
			builder.WriteString(s.RecentLogs[i])
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString("=== END CONTEXT ===\n")

	return builder.String()
}
