package dashboard

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/EasterCompany/dex-discord-service/services"
	"github.com/bwmarrin/discordgo"
)

// ServerDashboard shows Discord server/guild information
type ServerDashboard struct {
	session       *discordgo.Session
	logChannelID  string
	serverID      string
	cache         *MessageCache
	startTime     time.Time
	healthChecker *services.HealthChecker
}

// NewServerDashboard creates a new server dashboard
func NewServerDashboard(session *discordgo.Session, logChannelID, serverID string) *ServerDashboard {
	return &ServerDashboard{
		session:      session,
		logChannelID: logChannelID,
		serverID:     serverID,
		cache: &MessageCache{
			ThrottleDuration: 60 * time.Second, // Update at most once per 60 seconds
		},
		startTime: time.Now(),
	}
}

// Init creates the server dashboard message
func (d *ServerDashboard) Init() error {
	content := d.formatBootMessage()

	log.Println("[DASHBOARD_INIT] Creating Server dashboard...")

	msg, err := d.session.ChannelMessageSend(d.logChannelID, content)
	if err != nil {
		return fmt.Errorf("failed to create server dashboard: %w", err)
	}

	d.cache.MessageID = msg.ID
	d.cache.Content = content
	d.cache.LastUpdate = time.Now()
	d.cache.LastAPIUpdate = time.Now()

	log.Printf("[DASHBOARD_INIT] Server dashboard created: %s\n", msg.ID)

	return nil
}

// Update refreshes the server dashboard (throttled)
func (d *ServerDashboard) Update() error {
	content := d.formatServerInfo()
	return UpdateThrottled(d.cache, d.session, d.logChannelID, content)
}

// ForceUpdate bypasses throttle
func (d *ServerDashboard) ForceUpdate() error {
	content := d.formatServerInfo()
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, content)
}

// Finalize performs final update
func (d *ServerDashboard) Finalize() error {
	content := d.formatShutdownMessage()
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, content)
}

// formatBootMessage creates the initial boot message
func (d *ServerDashboard) formatBootMessage() string {
	return "**Server Dashboard**\n\n_Loading server information_"
}

// formatServerInfo creates the server information message
func (d *ServerDashboard) formatServerInfo() string {
	guild, err := d.session.Guild(d.serverID)
	if err != nil {
		return fmt.Sprintf("**Server Dashboard**\n\n❌ **Status:** Failed to fetch server info\n\n_Error: %v_", err)
	}

	owner, err := d.session.GuildMember(d.serverID, guild.OwnerID)
	if err != nil {
		return fmt.Sprintf("**Server Dashboard**\n\n❌ **Status:** Failed to fetch owner info\n\n_Error: %v_", err)
	}

	// Get role names
	var roleNames []string
	for _, roleID := range owner.Roles {
		role, err := d.session.State.Role(d.serverID, roleID)
		if err != nil {
			log.Printf("Failed to get role %s: %v", roleID, err)
			continue
		}
		roleNames = append(roleNames, role.Name)
	}

	var builder strings.Builder

	// Server info section
	builder.WriteString(fmt.Sprintf("**Server:** %s\n", guild.Name))
	builder.WriteString(fmt.Sprintf("**Server ID:** `%s`\n", guild.ID))

	// Owner info in single line
	ownerInfo := fmt.Sprintf("<@%s> (%s)", guild.OwnerID, owner.User.Username)
	if owner.Nick != "" {
		ownerInfo += fmt.Sprintf(" | %s", owner.Nick)
	}
	if len(roleNames) > 0 {
		ownerInfo += fmt.Sprintf(" | %s", strings.Join(roleNames, ", "))
	}
	builder.WriteString(fmt.Sprintf("**Owner:** %s\n\n", ownerInfo))

	// Dex-Net section
	if d.healthChecker != nil {
		builder.WriteString("**🏗️ Dex-Net**\n")
		builder.WriteString("```\n")

		allServices := d.healthChecker.GetAllServices()

		// Always show discord-service first (this service)
		builder.WriteString(fmt.Sprintf("%-25s %s %s\n",
			"discord-service",
			services.GetStatusEmoji("operational"),
			"127.0.0.1:8200",
		))

		// Show other services
		for name, status := range allServices {
			emoji := services.GetStatusEmoji(status.Status)
			endpoint := strings.TrimPrefix(status.Endpoint, "http://")

			// Display service with status
			line := fmt.Sprintf("%-25s %s %s",
				name,
				emoji,
				endpoint,
			)

			// Add response time if online
			if status.Status == "operational" || status.Status == "degraded" {
				line += fmt.Sprintf(" (%dms)", status.ResponseTime)
			}

			builder.WriteString(line + "\n")
		}

		builder.WriteString("```\n")
	}

	return builder.String()
}

// formatShutdownMessage creates the shutdown message
func (d *ServerDashboard) formatShutdownMessage() string {
	return "**Server Dashboard**\n\n_Bot shutting down_"
}

// GetServerID returns the server ID for context building
func (d *ServerDashboard) GetServerID() string {
	return d.serverID
}

// SetHealthChecker sets the health checker for service status display
func (d *ServerDashboard) SetHealthChecker(hc *services.HealthChecker) {
	d.healthChecker = hc
}
