// Package events handles Discord gateway events and dispatches them to appropriate handlers.
package events

import (
	"encoding/json"
	"fmt"
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
	key := h.DB.GenerateMessageCacheKey(guildID, channelID)

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

// Dummy event handlers - these ensure all Discord events are captured and state is updated
// Implement specific logic as needed

func (h *Handler) ChannelUpdate(s *discordgo.Session, c *discordgo.ChannelUpdate) {}

func (h *Handler) ChannelDelete(s *discordgo.Session, c *discordgo.ChannelDelete) {}

func (h *Handler) ChannelPinsUpdate(s *discordgo.Session, c *discordgo.ChannelPinsUpdate) {}

func (h *Handler) ThreadCreate(s *discordgo.Session, t *discordgo.ThreadCreate) {}

func (h *Handler) ThreadUpdate(s *discordgo.Session, t *discordgo.ThreadUpdate) {}

func (h *Handler) ThreadDelete(s *discordgo.Session, t *discordgo.ThreadDelete) {}

func (h *Handler) ThreadListSync(s *discordgo.Session, t *discordgo.ThreadListSync) {}

func (h *Handler) ThreadMemberUpdate(s *discordgo.Session, t *discordgo.ThreadMemberUpdate) {}

func (h *Handler) ThreadMembersUpdate(s *discordgo.Session, t *discordgo.ThreadMembersUpdate) {}

func (h *Handler) GuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {}

func (h *Handler) GuildUpdate(s *discordgo.Session, g *discordgo.GuildUpdate) {}

func (h *Handler) GuildDelete(s *discordgo.Session, g *discordgo.GuildDelete) {}

func (h *Handler) GuildBanAdd(s *discordgo.Session, g *discordgo.GuildBanAdd) {}

func (h *Handler) GuildBanRemove(s *discordgo.Session, g *discordgo.GuildBanRemove) {}

func (h *Handler) GuildMemberAdd(s *discordgo.Session, m *discordgo.GuildMemberAdd) {}

func (h *Handler) GuildMemberUpdate(s *discordgo.Session, m *discordgo.GuildMemberUpdate) {}

func (h *Handler) GuildMemberRemove(s *discordgo.Session, m *discordgo.GuildMemberRemove) {}

func (h *Handler) GuildRoleCreate(s *discordgo.Session, r *discordgo.GuildRoleCreate) {}

func (h *Handler) GuildRoleUpdate(s *discordgo.Session, r *discordgo.GuildRoleUpdate) {}

func (h *Handler) GuildRoleDelete(s *discordgo.Session, r *discordgo.GuildRoleDelete) {}

func (h *Handler) GuildEmojisUpdate(s *discordgo.Session, e *discordgo.GuildEmojisUpdate) {}

func (h *Handler) GuildStickersUpdate(s *discordgo.Session, e *discordgo.GuildStickersUpdate) {}

func (h *Handler) GuildMembersChunk(s *discordgo.Session, m *discordgo.GuildMembersChunk) {}

func (h *Handler) GuildIntegrationsUpdate(s *discordgo.Session, i *discordgo.GuildIntegrationsUpdate) {
}

func (h *Handler) StageInstanceEventCreate(s *discordgo.Session, e *discordgo.StageInstanceEventCreate) {
}

func (h *Handler) StageInstanceEventUpdate(s *discordgo.Session, e *discordgo.StageInstanceEventUpdate) {
}

func (h *Handler) StageInstanceEventDelete(s *discordgo.Session, e *discordgo.StageInstanceEventDelete) {
}

func (h *Handler) GuildScheduledEventCreate(s *discordgo.Session, e *discordgo.GuildScheduledEventCreate) {
}

func (h *Handler) GuildScheduledEventUpdate(s *discordgo.Session, e *discordgo.GuildScheduledEventUpdate) {
}

func (h *Handler) GuildScheduledEventDelete(s *discordgo.Session, e *discordgo.GuildScheduledEventDelete) {
}

func (h *Handler) GuildScheduledEventUserAdd(s *discordgo.Session, e *discordgo.GuildScheduledEventUserAdd) {
}

func (h *Handler) GuildScheduledEventUserRemove(s *discordgo.Session, e *discordgo.GuildScheduledEventUserRemove) {
}

func (h *Handler) IntegrationCreate(s *discordgo.Session, i *discordgo.IntegrationCreate) {}

func (h *Handler) IntegrationUpdate(s *discordgo.Session, i *discordgo.IntegrationUpdate) {}

func (h *Handler) IntegrationDelete(s *discordgo.Session, i *discordgo.IntegrationDelete) {}

func (h *Handler) MessageUpdate(s *discordgo.Session, m *discordgo.MessageUpdate) {}

func (h *Handler) MessageDelete(s *discordgo.Session, m *discordgo.MessageDelete) {}

func (h *Handler) MessageReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {}

func (h *Handler) MessageReactionRemove(s *discordgo.Session, r *discordgo.MessageReactionRemove) {}

func (h *Handler) MessageReactionRemoveAll(s *discordgo.Session, r *discordgo.MessageReactionRemoveAll) {
}

func (h *Handler) PresenceUpdate(s *discordgo.Session, p *discordgo.PresenceUpdate) {}

func (h *Handler) Resumed(s *discordgo.Session, r *discordgo.Resumed) {}

func (h *Handler) TypingStart(s *discordgo.Session, t *discordgo.TypingStart) {}

func (h *Handler) UserUpdate(s *discordgo.Session, u *discordgo.UserUpdate) {}

func (h *Handler) VoiceServerUpdate(s *discordgo.Session, v *discordgo.VoiceServerUpdate) {}

func (h *Handler) VoiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {}

func (h *Handler) MessageDeleteBulk(s *discordgo.Session, m *discordgo.MessageDeleteBulk) {}

func (h *Handler) WebhooksUpdate(s *discordgo.Session, w *discordgo.WebhooksUpdate) {}

func (h *Handler) InteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {}

func (h *Handler) InviteCreate(s *discordgo.Session, i *discordgo.InviteCreate) {}

func (h *Handler) InviteDelete(s *discordgo.Session, i *discordgo.InviteDelete) {}

func (h *Handler) ApplicationCommandPermissionsUpdate(s *discordgo.Session, a *discordgo.ApplicationCommandPermissionsUpdate) {
}

func (h *Handler) AutoModerationRuleCreate(s *discordgo.Session, a *discordgo.AutoModerationRuleCreate) {
}

func (h *Handler) AutoModerationRuleUpdate(s *discordgo.Session, a *discordgo.AutoModerationRuleUpdate) {
}

func (h *Handler) AutoModerationRuleDelete(s *discordgo.Session, a *discordgo.AutoModerationRuleDelete) {
}

func (h *Handler) AutoModerationActionExecution(s *discordgo.Session, a *discordgo.AutoModerationActionExecution) {
}

func (h *Handler) GuildAuditLogEntryCreate(s *discordgo.Session, a *discordgo.GuildAuditLogEntryCreate) {
}

func (h *Handler) MessagePollVoteAdd(s *discordgo.Session, p *discordgo.MessagePollVoteAdd) {}

func (h *Handler) MessagePollVoteRemove(s *discordgo.Session, p *discordgo.MessagePollVoteRemove) {}

func (h *Handler) EntitlementCreate(s *discordgo.Session, e *discordgo.EntitlementCreate) {}

func (h *Handler) EntitlementUpdate(s *discordgo.Session, e *discordgo.EntitlementUpdate) {}

func (h *Handler) EntitlementDelete(s *discordgo.Session, e *discordgo.EntitlementDelete) {}

func (h *Handler) SubscriptionCreate(s *discordgo.Session, sub *discordgo.SubscriptionCreate) {}

func (h *Handler) SubscriptionUpdate(s *discordgo.Session, sub *discordgo.SubscriptionUpdate) {}

func (h *Handler) SubscriptionDelete(s *discordgo.Session, sub *discordgo.SubscriptionDelete) {}

func (h *Handler) Connect(s *discordgo.Session, c *discordgo.Connect) {}

func (h *Handler) Disconnect(s *discordgo.Session, d *discordgo.Disconnect) {}

func (h *Handler) RateLimit(s *discordgo.Session, r *discordgo.RateLimit) {}
