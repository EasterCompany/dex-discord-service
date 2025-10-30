package events

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type CommandHandlerFunc func(h *Handler, s *discordgo.Session, m *discordgo.MessageCreate)

var commandHandlers = map[string]CommandHandlerFunc{
	"join":      (*Handler).joinVoice,
	"leave":     (*Handler).leaveVoice,
	"clear_dex": (*Handler).clearDex,
}

func (h *Handler) handleUnknownCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	msg, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("`%s` is not a valid command.", m.Content))
	if err != nil {
		return
	}
	time.Sleep(10 * time.Second)
	_ = s.ChannelMessageDelete(msg.ChannelID, msg.ID)
}

func (h *Handler) clearDex(s *discordgo.Session, m *discordgo.MessageCreate) {
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

func (h *Handler) routeCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Delete the command message, but only if it's not a DM.
	if m.GuildID != "" {
		_ = s.ChannelMessageDelete(m.ChannelID, m.ID)
	}

	parts := strings.Fields(m.Content)
	if len(parts) == 0 {
		return
	}

	command := strings.TrimPrefix(parts[0], "!")
	if handler, ok := commandHandlers[command]; ok {
		handler(h, s, m)
	} else {
		h.handleUnknownCommand(s, m)
	}
}

func (h *Handler) joinVoice(s *discordgo.Session, m *discordgo.MessageCreate) {
	h.disconnectFromVoice(s, m.GuildID)
	time.Sleep(1 * time.Second)

	g, err := s.State.Guild(m.GuildID)
	if err != nil {
		return
	}
	for _, vs := range g.VoiceStates {
		if vs.UserID == m.Author.ID {
			state := h.StateManager.GetOrStoreGuildState(m.GuildID)
			if h.DB != nil {
				if err := h.DB.SaveGuildState(m.GuildID, state); err != nil {
					h.Logger.Error(fmt.Sprintf("Error saving guild state for guild %s", m.GuildID), err)
				}
			}
			channel, err := s.Channel(vs.ChannelID)
			if err != nil {
				return
			}
			msg, _ := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Connecting to %s (%s) at %s (%s).", channel.Name, channel.ID, g.Name, g.ID))
			if msg != nil {
				state.ConnectionMessageID = msg.ID
				state.ConnectionMessageChannelID = msg.ChannelID
				state.ConnectionStartTime = time.Now()
			}

			const maxRetries = 3
			var vc *discordgo.VoiceConnection

			for i := 0; i < maxRetries; i++ {
				vc, err = s.ChannelVoiceJoin(m.GuildID, vs.ChannelID, false, false)
				if err == nil {
					break
				}
				retrySeconds := int(math.Pow(2, float64(i)))
				if msg != nil {
					_, _ = s.ChannelMessageEdit(m.ChannelID, msg.ID, fmt.Sprintf("Failed to connect, retrying in %d seconds...", retrySeconds))
				}
				time.Sleep(time.Duration(retrySeconds) * time.Second)
			}

			if err != nil {
				if msg != nil {
					_, _ = s.ChannelMessageEdit(m.ChannelID, msg.ID, fmt.Sprintf("Failed to connect to %s (%s) at %s (%s).", channel.Name, channel.ID, g.Name, g.ID))
				}
				h.disconnectFromVoice(s, m.GuildID)
				return
			}

			vc.AddHandler(func(vc *discordgo.VoiceConnection, p *discordgo.VoiceSpeakingUpdate) {
				h.SpeakingUpdate(vc, p)
			})
			state.ConnectionChannelID = vc.ChannelID
			go h.handleVoice(s, vc, state)
			return
		}
	}
	_, _ = s.ChannelMessageSend(m.ChannelID, "You need to be in a voice channel for me to join!")
}

func (h *Handler) leaveVoice(s *discordgo.Session, m *discordgo.MessageCreate) {
	h.disconnectFromVoice(s, m.GuildID)
}

func (h *Handler) disconnectFromVoice(s *discordgo.Session, guildID string) {
	if vc, ok := s.VoiceConnections[guildID]; ok {
		state, ok := h.StateManager.GetGuildState(guildID)
		if !ok {
			_ = vc.Disconnect()
			return
		}

		// Cancel the context to signal the handleVoice goroutine to exit
		if state.CancelFunc != nil {
			state.CancelFunc()
		}

		// Disconnect from voice
		_ = vc.Disconnect()

		state.Mutex.Lock()
		defer state.Mutex.Unlock()

		if state.ConnectionMessageID != "" {
			duration := time.Since(state.ConnectionStartTime).Round(time.Second)
			channel, _ := s.Channel(state.ConnectionChannelID)
			g, _ := s.State.Guild(guildID)

			var editContent string
			if channel != nil && g != nil {
				// Count unique users who spoke
				uniqueUsers := make(map[string]bool)
				for _, userID := range state.SSRCUserMap {
					uniqueUsers[userID] = true
				}

				editContent = fmt.Sprintf("**Disconnected from %s (%s) at %s (%s)**\n**Duration:** %s\n**Users tracked:** %d\n**Total SSRCs:** %d",
					channel.Name, channel.ID, g.Name, g.ID, duration, len(uniqueUsers), len(state.SSRCUserMap))

				// Add warning about unmapped SSRCs if any
				if len(state.UnmappedSSRCs) > 0 {
					editContent += fmt.Sprintf("\n⚠️ **Unmapped SSRCs:** %d (users who joined before bot)", len(state.UnmappedSSRCs))
				}
			} else {
				editContent = fmt.Sprintf("Disconnected after %s.", duration)
			}
			_, _ = s.ChannelMessageEdit(state.ConnectionMessageChannelID, state.ConnectionMessageID, editContent)
		}

		for ssrc, stream := range state.ActiveStreams {
			h.finalizeStream(s, guildID, ssrc, stream)
			delete(state.ActiveStreams, ssrc)
		}
		h.StateManager.DeleteGuildState(guildID)
	}
}
