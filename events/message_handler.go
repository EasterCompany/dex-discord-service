// Package events handles Discord gateway events and dispatches them to appropriate handlers.
package events

import (
	"context"
	"fmt"
	"strings"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/llm"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/bwmarrin/discordgo"
)

type MessageHandler struct {
	DB             cache.Cache
	LLMClient      *llm.Client
	UserManager    *UserManager
	Logger         logger.Logger
	Session        *discordgo.Session
	CommandHandler *CommandHandler
}

func NewMessageHandler(db cache.Cache, llmClient *llm.Client, userManager *UserManager, logger logger.Logger, session *discordgo.Session, commandHandler *CommandHandler) *MessageHandler {
	return &MessageHandler{
		DB:             db,
		LLMClient:      llmClient,
		UserManager:    userManager,
		Logger:         logger,
		Session:        session,
		CommandHandler: commandHandler,
	}
}

func (h *MessageHandler) Handle(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Check if the message is a command
	if strings.HasPrefix(m.Content, "!") {
		h.CommandHandler.RouteCommand(s, m)
		return
	}

	if h.DB != nil {
		if m.GuildID == "" {
			if err := h.DB.AddDMChannel(m.ChannelID); err != nil {
				h.Logger.Error("Error adding DM channel to cache", err)
			}
		}
		key := h.DB.GenerateMessageCacheKey(m.GuildID, m.ChannelID)
		if err := h.DB.AddMessage(key, m.Message); err != nil {
			h.Logger.Error(fmt.Sprintf("Error saving message %s", m.ID), err)
		}

		if h.LLMClient != nil {
			go h.ProcessLLMResponse(s, m)
		}
	}
}

func (h *MessageHandler) ProcessLLMResponse(s *discordgo.Session, m *discordgo.MessageCreate) {
	userState := h.UserManager.GetOrCreateUserState(m.Author.ID)
	userState.TransitionToPending(s, m.ChannelID)

	key := h.DB.GenerateMessageCacheKey(m.GuildID, m.ChannelID)
	last50Messages, err := h.DB.GetLastNMessages(key, 50)
	if err != nil {
		h.Logger.Error("Failed to get engagement history from cache", err)
		userState.TransitionToIdle()
		return
	}

	var engagementHistory []*discordgo.Message
	for i := len(last50Messages) - 1; i >= 0; i-- {
		msg := last50Messages[i]
		if msg.Author.ID == s.State.User.ID {
			break
		}
		engagementHistory = append(engagementHistory, msg)
		if len(engagementHistory) >= 5 {
			break
		}
	}

	for i, j := 0, len(engagementHistory)-1; i < j; i, j = i+1, j-1 {
		engagementHistory[i], engagementHistory[j] = engagementHistory[j], engagementHistory[i]
	}

	decision, arg, err := h.LLMClient.GetEngagementDecision(s, m, engagementHistory)
	if err != nil {
		h.Logger.Error("Failed to check LLM engagement", err)
		userState.TransitionToIdle()
		return
	}

	h.Logger.Info(fmt.Sprintf("Engagement decision for message '%s': %s, arg: %s", m.Content, decision, arg))

	switch decision {
	case "REPLY":
		ctx := userState.TransitionToStreaming()
		defer func() {
			userState.TransitionToIdle()
			if ctx.Err() != context.Canceled {
				userState.ClearInterruptedState()
			}
		}()

		fullHistory, err := h.DB.GetLastNMessages(key, 10)
		if err != nil {
			h.Logger.Error("Failed to get conversation history from cache", err)
			return
		}

		contextBlock, err := h.LLMClient.GenerateContextBlock(s, m)
		if err != nil {
			h.Logger.Error("Failed to generate LLM context block", err)
			return
		}

		userState.SaveInterruptedState(fullHistory, contextBlock)

		responseMessage, err := h.LLMClient.StreamChatCompletion(ctx, s, m.Message, fullHistory, contextBlock)
		if err != nil {
			if err != context.Canceled {
				h.Logger.Error("Failed to stream LLM chat completion", err)
			}
			return
		}
		userState.Mutex.Lock()
		userState.MessageID = responseMessage.ID
		userState.ChannelID = responseMessage.ChannelID
		userState.Mutex.Unlock()

	case "REACT":
		userState.TransitionToIdle()
		if arg != "" {
			_ = s.MessageReactionAdd(m.ChannelID, m.ID, arg)
		}
	case "STOP":
		userState.TransitionToIdle()
	case "CONTINUE":
		userState.Mutex.Lock()
		interruptedHistory := userState.InterruptedHistory
		interruptedContext := userState.InterruptedContext
		userState.Mutex.Unlock()

		if interruptedHistory == nil {
			userState.TransitionToIdle()
			return
		}

		ctx := userState.TransitionToStreaming()
		defer func() {
			userState.TransitionToIdle()
			userState.ClearInterruptedState()
		}()

		responseMessage, err := h.LLMClient.StreamChatCompletion(ctx, s, m.Message, interruptedHistory, interruptedContext)
		if err != nil {
			if err != context.Canceled {
				h.Logger.Error("Failed to stream LLM chat completion", err)
			}
			return
		}
		userState.Mutex.Lock()
		userState.MessageID = responseMessage.ID
		userState.ChannelID = responseMessage.ChannelID
		userState.Mutex.Unlock()

	case "IGNORE":
		userState.TransitionToIdle()
	}
}
