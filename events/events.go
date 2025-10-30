// Package events handles Discord gateway events and dispatches them to appropriate handlers.
package events

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/interfaces"
	"github.com/EasterCompany/dex-discord-interface/llm"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/bwmarrin/discordgo"
)

type Handler struct {
	DB           cache.Cache
	DiscordCfg   *config.DiscordConfig
	BotCfg       *config.BotConfig
	Session      *discordgo.Session
	Logger       logger.Logger
	StateManager *StateManager
	UserManager  *UserManager
	SttClient    interfaces.SpeechToText
	LLMClient    *llm.Client
}

func NewHandler(db cache.Cache, discordCfg *config.DiscordConfig, botCfg *config.BotConfig, s *discordgo.Session, logger logger.Logger, stateManager *StateManager, userManager *UserManager, sttClient interfaces.SpeechToText, llmClient *llm.Client) *Handler {
	return &Handler{
		DB:           db,
		DiscordCfg:   discordCfg,
		BotCfg:       botCfg,
		Session:      s,
		Logger:       logger,
		StateManager: stateManager,
		UserManager:  userManager,
		SttClient:    sttClient,
		LLMClient:    llmClient,
	}
}

func (h *Handler) GenerateMessageCacheKey(guildID, channelID string) string {
	if guildID == "" {
		return fmt.Sprintf("messages:dm:%s", channelID)
	}
	return fmt.Sprintf("messages:guild:%s:channel:%s", guildID, channelID)
}

func (h *Handler) GenerateAudioCacheKey(filename string) string {
	return fmt.Sprintf("audio:%s", filename)
}

func (h *Handler) Ready(s *discordgo.Session, r *discordgo.Ready) (int, int64) {
	h.Logger.Post("Connection established. Starting initial message history sync...")
	var wg sync.WaitGroup
	results := make(chan struct {
		count int
		size  int64
	}, len(r.Guilds)+len(r.PrivateChannels))

	for _, g := range r.Guilds {
		channels, err := s.GuildChannels(g.ID)
		if err != nil {
			h.Logger.Error(fmt.Sprintf("Failed to get channels for guild %s", g.ID), err)
			continue
		}
		for _, c := range channels {
			if c.Type == discordgo.ChannelTypeGuildText || c.Type == discordgo.ChannelTypeGuildVoice {
				wg.Add(1)
				go func(guildID, channelID string) {
					defer wg.Done()
					count, size := h.fetchAndStoreLast50Messages(s, guildID, channelID)
					results <- struct {
						count int
						size  int64
					}{count, size}
				}(c.GuildID, c.ID)
			}
		}
	}
	for _, c := range r.PrivateChannels {
		wg.Add(1)
		go func(guildID, channelID string) {
			defer wg.Done()
			count, size := h.fetchAndStoreLast50Messages(s, guildID, channelID)
			results <- struct {
				count int
				size  int64
			}{count, size}
		}("", c.ID)
	}

	wg.Wait()
	close(results)

	totalMessages := 0
	var totalSize int64
	for res := range results {
		totalMessages += res.count
		totalSize += res.size
	}

	h.Logger.Post("Initial message sync process initiated.")
	return totalMessages, totalSize
}

func (h *Handler) fetchAndStoreLast50Messages(s *discordgo.Session, guildID, channelID string) (int, int64) {
	if h.DB == nil {
		return 0, 0
	}
	messages, err := s.ChannelMessages(channelID, 50, "", "", "")
	if err != nil {
		h.Logger.Error(fmt.Sprintf("Failed to fetch messages for channel %s", channelID), err)
		return 0, 0
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	key := h.GenerateMessageCacheKey(guildID, channelID)

	var totalSize int64
	for _, m := range messages {
		jsonMsg, err := json.Marshal(m)
		if err != nil {
			continue
		}
		totalSize += int64(len(jsonMsg))
	}

	if err := h.DB.BulkInsertMessages(key, messages); err != nil {
		h.Logger.Error(fmt.Sprintf("Failed to bulk insert messages for channel %s", channelID), err)
		return 0, 0
	}
	return len(messages), totalSize
}

func (h *Handler) ChannelCreate(s *discordgo.Session, c *discordgo.ChannelCreate) {
	// This handler is intentionally left empty.
	// Registering this handler is enough for discordgo to process the event and update the state.
}

func (h *Handler) MessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Check if the message is a command
	if strings.HasPrefix(m.Content, "!") {
		h.routeCommand(s, m)
		return
	}

	userState := h.UserManager.GetOrCreateUserState(m.Author.ID)
	userState.Mutex.Lock()

	switch userState.State {
	case StatePending:
		// If a response is pending, cancel it and let the new message trigger a new response.
		if userState.Timer != nil {
			userState.Timer.Stop()
		}
		userState.State = StateIdle
	case StateStreaming:
		// If a response is streaming, cancel it and delete the partial message.
		if userState.CancelFunc != nil {
			userState.CancelFunc()
		}
		_ = s.ChannelMessageDelete(userState.ChannelID, userState.MessageID)
		userState.State = StateIdle
	}

	userState.Mutex.Unlock()

	if h.DB != nil {
		if m.GuildID == "" {
			if err := h.DB.AddDMChannel(m.ChannelID); err != nil {
				h.Logger.Error("Error adding DM channel to cache", err)
			}
		}
		key := h.GenerateMessageCacheKey(m.GuildID, m.ChannelID)
		if err := h.DB.AddMessage(key, m.Message); err != nil {
			h.Logger.Error(fmt.Sprintf("Error saving message %s", m.ID), err)
		}

		if h.LLMClient != nil {
			go h.processLLMResponse(s, m)
		}
	}
}
