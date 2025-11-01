package commands

import (
	"fmt"
	"log"
	"strings"

	"github.com/EasterCompany/dex-discord-service/cache"
	"github.com/EasterCompany/dex-discord-service/config"
	"github.com/EasterCompany/dex-discord-service/dashboard"
	"github.com/bwmarrin/discordgo"
)

// Handler manages all bot commands
type Handler struct {
	session           *discordgo.Session
	config            *config.Config
	permissionChecker *PermissionChecker
	dashboardManager  *dashboard.Manager
	redisClient       *cache.RedisClient
	logChannelID      string
}

// NewHandler creates a new command handler
func NewHandler(
	session *discordgo.Session,
	cfg *config.Config,
	dashboardManager *dashboard.Manager,
	redisClient *cache.RedisClient,
) *Handler {
	return &Handler{
		session:           session,
		config:            cfg,
		permissionChecker: NewPermissionChecker(cfg, cfg.ServerID),
		dashboardManager:  dashboardManager,
		redisClient:       redisClient,
		logChannelID:      cfg.LogChannelID,
	}
}

// HandleCommand processes incoming commands
func (h *Handler) HandleCommand(m *discordgo.MessageCreate) {
	// Ignore bot messages
	if m.Author.Bot {
		return
	}

	// Only process commands that start with /
	if !strings.HasPrefix(m.Content, "/") {
		return
	}

	// Check permissions
	if !h.permissionChecker.CanExecuteCommand(h.session, m.Author.ID) {
		h.sendResponse(m.ChannelID, "❌ You do not have permission to execute commands.")
		log.Printf("[COMMAND] Permission denied for user %s (%s)", m.Author.Username, m.Author.ID)
		return
	}

	// Parse command and arguments
	parts := strings.Fields(m.Content)
	command := strings.TrimPrefix(parts[0], "/")
	args := parts[1:]

	log.Printf("[COMMAND] User %s (%s) executed: %s with args: %v",
		m.Author.Username, m.Author.ID, command, args)

	// Route to appropriate command handler
	switch command {
	case "update":
		h.handleUpdate(m.ChannelID, args)
	case "join":
		h.handleJoin(m.ChannelID, args)
	case "sleep":
		h.handleSleep(m.ChannelID)
	case "tasks":
		h.handleTasks(m.ChannelID)
	case "help":
		h.handleHelp(m.ChannelID)
	default:
		h.sendResponse(m.ChannelID, fmt.Sprintf("❌ Unknown command: `/%s`. Use `/help` for available commands.", command))
	}
}

// sendResponse sends a message to a channel
func (h *Handler) sendResponse(channelID, message string) {
	_, err := h.session.ChannelMessageSend(channelID, message)
	if err != nil {
		log.Printf("[COMMAND] Failed to send response: %v", err)
	}
}

// handleHelp shows available commands
func (h *Handler) handleHelp(channelID string) {
	help := "**Available Commands:**\n" +
		"`/update` - Force update all dashboards from cache (no API calls)\n" +
		"`/update git` - Check for git updates and report status\n" +
		"`/join <channel>` - Join a voice channel (smart matching)\n" +
		"`/sleep` - Enter sleep mode (disconnect from voice, pause updates)\n" +
		"`/tasks` - Show current task queue\n" +
		"`/help` - Show this help message"

	h.sendResponse(channelID, help)
}
