package events

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/EasterCompany/dex-discord-interface/config"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/bwmarrin/discordgo"
)

type CommandHandler struct {
	Session      *discordgo.Session
	Logger       logger.Logger
	VoiceHandler *VoiceHandler
	DiscordCfg   *config.DiscordConfig
}

func NewCommandHandler(s *discordgo.Session, logger logger.Logger, voiceHandler *VoiceHandler, discordCfg *config.DiscordConfig) *CommandHandler {
	return &CommandHandler{
		Session:      s,
		Logger:       logger,
		VoiceHandler: voiceHandler,
		DiscordCfg:   discordCfg,
	}
}

func (h *CommandHandler) RouteCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Delete the command message, but only if it's not a DM.
	if m.GuildID != "" {
		log.Printf("[DISCORD_DELETE] ChannelID: %s | MessageID: %s | Command: %s\n", m.ChannelID, m.ID, m.Content)
		_ = s.ChannelMessageDelete(m.ChannelID, m.ID)
	}

	parts := strings.Fields(m.Content)
	if len(parts) == 0 {
		return
	}

	command := strings.TrimPrefix(parts[0], "!")
	switch command {
	case "join":
		h.VoiceHandler.JoinVoice(s, m)
	case "leave":
		h.VoiceHandler.LeaveVoice(s, m)
	case "clear_dex":
		h.clearDex(s, m)
	default:
		h.handleUnknownCommand(s, m)
	}
}

func (h *CommandHandler) handleUnknownCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	errMsg := fmt.Sprintf("Unknown command from %s: `%s` is not a valid command.", m.Author.Username, m.Content)
	log.Printf("[DISCORD_POST] %s\n", errMsg)
	msg, err := s.ChannelMessageSend(h.DiscordCfg.LogChannelID, errMsg)
	if err != nil {
		return
	}
	time.Sleep(10 * time.Second)
	log.Printf("[DISCORD_DELETE] ChannelID: %s | MessageID: %s | Auto-delete error message\n", msg.ChannelID, msg.ID)
	_ = s.ChannelMessageDelete(msg.ChannelID, msg.ID)
}

func (h *CommandHandler) clearDex(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID == "" {
		// In DMs, the bot cannot delete messages. Provide feedback to the user.
		dmMsg := fmt.Sprintf("Cannot clear messages in DM from %s: API limitations.", m.Author.Username)
		log.Printf("[DISCORD_POST] %s\n", dmMsg)
		_, _ = s.ChannelMessageSend(h.DiscordCfg.LogChannelID, dmMsg)
		return
	}
	botID := s.State.User.ID

	messages, err := s.ChannelMessages(m.ChannelID, 100, "", "", "")
	if err != nil {
		h.Logger.Error("Failed to fetch messages", err)
		return
	}

	deletedCount := 0
	for _, msg := range messages {
		if msg.Author.ID == botID {
			log.Printf("[DISCORD_DELETE] ChannelID: %s | MessageID: %s | clear_dex command\n", msg.ChannelID, msg.ID)
			_ = s.ChannelMessageDelete(msg.ChannelID, msg.ID)
			deletedCount++
		}
	}

	clearMsg := fmt.Sprintf("Cleared %d bot messages from channel %s requested by %s", deletedCount, m.ChannelID, m.Author.Username)
	log.Printf("[INFO] %s\n", clearMsg)
}
