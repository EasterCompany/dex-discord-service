package events

import (
	"fmt"
	"strings"
	"time"

	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/bwmarrin/discordgo"
)

type CommandHandler struct {
	Session      *discordgo.Session
	Logger       logger.Logger
	VoiceHandler *VoiceHandler
}

func NewCommandHandler(s *discordgo.Session, logger logger.Logger, voiceHandler *VoiceHandler) *CommandHandler {
	return &CommandHandler{
		Session:      s,
		Logger:       logger,
		VoiceHandler: voiceHandler,
	}
}

func (h *CommandHandler) RouteCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Delete the command message, but only if it's not a DM.
	if m.GuildID != "" {
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
	msg, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("`%s` is not a valid command.", m.Content))
	if err != nil {
		return
	}
	time.Sleep(10 * time.Second)
	_ = s.ChannelMessageDelete(msg.ChannelID, msg.ID)
}

func (h *CommandHandler) clearDex(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID == "" {
		// In DMs, the bot cannot delete messages. Provide feedback to the user.
		_, _ = s.ChannelMessageSend(m.ChannelID, "I cannot delete messages in Direct Messages due to Discord API limitations.")
		return
	}
	botID := s.State.User.ID

	messages, err := s.ChannelMessages(m.ChannelID, 100, "", "", "")
	if err != nil {
		h.Logger.Error("Failed to fetch messages", err)
		return
	}

	for _, msg := range messages {
		if msg.Author.ID == botID {
			_ = s.ChannelMessageDelete(msg.ChannelID, msg.ID)
		}
	}
}
