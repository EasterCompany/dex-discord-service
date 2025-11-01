package dashboard

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// ServerDashboard shows Discord server/guild information
type ServerDashboard struct {
	session      *discordgo.Session
	logChannelID string
	serverID     string
	cache        *MessageCache
	startTime    time.Time
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

	return fmt.Sprintf("**Server:** %s\n"+
		"**Server ID:** `%s`\n"+
		"**Owner:** <@%s> (%s) | %s\n"+
		"**Roles:** %s\n\n",
		guild.Name,
		guild.ID,
		guild.OwnerID,
		owner.User.Username,
		owner.Nick,
		strings.Join(roleNames, ", "),
	)
}

// formatShutdownMessage creates the shutdown message
func (d *ServerDashboard) formatShutdownMessage() string {
	return "**Server Dashboard**\n\n_Bot shutting down_"
}
