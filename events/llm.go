package events

import (
	"github.com/bwmarrin/discordgo"
)

func (h *Handler) processLLMResponse(s *discordgo.Session, m *discordgo.MessageCreate) {
	key := h.GenerateMessageCacheKey(m.GuildID, m.ChannelID)
	engagementHistory, err := h.DB.GetLastNMessages(key, 10)
	if err != nil {
		h.Logger.Error("Failed to get engagement history from cache", err)
		return
	}

	shouldEngage, err := h.LLMClient.ShouldEngage(s, m, engagementHistory)
	if err != nil {
		h.Logger.Error("Failed to check LLM engagement", err)
		return
	}

	if !shouldEngage {
		return
	}

	fullHistory, err := h.DB.GetLastNMessages(key, 50)
	if err != nil {
		h.Logger.Error("Failed to get conversation history from cache", err)
		return
	}

	contextBlock, err := h.LLMClient.GenerateContextBlock(s, m)
	if err != nil {
		h.Logger.Error("Failed to generate LLM context block", err)
		return
	}

	_ = s.ChannelTyping(m.ChannelID)

	if err := h.LLMClient.StreamChatCompletion(s, m.Message, fullHistory, contextBlock); err != nil {
		h.Logger.Error("Failed to stream LLM chat completion", err)
	}
}
