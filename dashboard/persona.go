package dashboard

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/EasterCompany/dex-discord-service/config"
	"github.com/bwmarrin/discordgo"
)

// PersonaDashboard displays the server's bot persona configuration
type PersonaDashboard struct {
	session      *discordgo.Session
	logChannelID string
	config       *config.Config
	cache        *MessageCache
}

// NewPersonaDashboard creates a new persona dashboard
func NewPersonaDashboard(session *discordgo.Session, logChannelID string, cfg *config.Config) *PersonaDashboard {
	return &PersonaDashboard{
		session:      session,
		logChannelID: logChannelID,
		config:       cfg,
		cache: &MessageCache{
			ThrottleDuration: 60 * time.Second, // Update at most once per 60 seconds
		},
	}
}

// Init creates the persona dashboard message
func (d *PersonaDashboard) Init() error {
	content := d.formatBootMessage()

	log.Println("[DASHBOARD_INIT] Creating Persona dashboard...")

	msg, err := d.session.ChannelMessageSend(d.logChannelID, content)
	if err != nil {
		return fmt.Errorf("failed to create persona dashboard: %w", err)
	}

	d.cache.MessageID = msg.ID
	d.cache.Content = content
	d.cache.LastUpdate = time.Now()
	d.cache.LastAPIUpdate = time.Now()

	log.Printf("[DASHBOARD_INIT] Persona dashboard created: %s\n", msg.ID)

	return nil
}

// Update refreshes the persona dashboard (throttled)
func (d *PersonaDashboard) Update() error {
	content := d.formatPersonaInfo()
	return UpdateThrottled(d.cache, d.session, d.logChannelID, content)
}

// ForceUpdate bypasses throttle
func (d *PersonaDashboard) ForceUpdate() error {
	content := d.formatPersonaInfo()
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, content)
}

// Finalize performs final update
func (d *PersonaDashboard) Finalize() error {
	content := d.formatShutdownMessage()
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, content)
}

// formatBootMessage creates the initial boot message
func (d *PersonaDashboard) formatBootMessage() string {
	return "**Server Persona**\n\n_Loading persona configuration..._"
}

// formatPersonaInfo creates the persona information message
func (d *PersonaDashboard) formatPersonaInfo() string {
	persona := d.config.ServerPersona

	var builder strings.Builder

	builder.WriteString("**🎭 Server Persona**\n\n")

	// Status indicator
	if persona.Enabled {
		builder.WriteString("**Status:** ✅ Enabled\n\n")
	} else {
		builder.WriteString("**Status:** ❌ Disabled\n\n")
		builder.WriteString("_Persona configuration is disabled. Enable it in the config to customize bot behavior._\n")
		return builder.String()
	}

	// Persona name
	if persona.Name != "" {
		builder.WriteString(fmt.Sprintf("**Name:** %s\n", persona.Name))
	}

	// Personality
	if persona.Personality != "" {
		builder.WriteString(fmt.Sprintf("**Personality:** %s\n\n", persona.Personality))
	} else {
		builder.WriteString("\n")
	}

	// Response style
	if persona.ResponseStyle != "" {
		builder.WriteString(fmt.Sprintf("**Response Style:** %s\n\n", persona.ResponseStyle))
	}

	// Behavior rules
	if len(persona.BehaviorRules) > 0 {
		builder.WriteString("**Behavior Rules:**\n")
		for i, rule := range persona.BehaviorRules {
			builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, rule))
		}
		builder.WriteString("\n")
	}

	// System prompt indicator
	if persona.SystemPrompt != "" {
		builder.WriteString("**Custom System Prompt:** ✓ Configured\n")
	}

	builder.WriteString("\n_Configuration managed via `~/Dexter/config/dex-discord-service.json`_")

	return builder.String()
}

// formatShutdownMessage creates the shutdown message
func (d *PersonaDashboard) formatShutdownMessage() string {
	return "**Server Persona**\n\n_Bot shutting down_"
}

// UpdateConfig updates the config reference (useful for hot-reload)
func (d *PersonaDashboard) UpdateConfig(cfg *config.Config) {
	d.config = cfg
}
