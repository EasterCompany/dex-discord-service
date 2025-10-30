
package events

import (
	"context"
	"fmt"
	"time"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/llm"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/bwmarrin/discordgo"
)

type LLMHandler struct {
	DB          cache.Cache
	Logger      logger.Logger
	UserManager *UserManager
	LLMClient   *llm.Client
}

func NewLLMHandler(db cache.Cache, logger logger.Logger, userManager *UserManager, llmClient *llm.Client) *LLMHandler {
	return &LLMHandler{
		DB:          db,
		Logger:      logger,
		UserManager: userManager,
		LLMClient:   llmClient,
	}
}

func (h *LLMHandler) ProcessLLMResponse(s *discordgo.Session, m *discordgo.MessageCreate) {
	userState := h.UserManager.GetOrCreateUserState(m.Author.ID)
	userState.Mutex.Lock()
	if userState.State != StateIdle {
		if userState.CancelFunc != nil {
			userState.CancelFunc()
		}
		if userState.Timer != nil {
			userState.Timer.Stop()
		}
		userState.State = StateIdle
	}
	userState.State = StatePending

	// Start a ticker to keep the typing indicator alive
	ticker := time.NewTicker(8 * time.Second)
	userState.Timer = ticker
	go func() {
		for range ticker.C {
			_ = s.ChannelTyping(m.ChannelID)
		}
	}()

	userState.Mutex.Unlock()

	key := fmt.Sprintf("messages:guild:%s:channel:%s", m.GuildID, m.ChannelID)
	if m.GuildID == "" {
		key = fmt.Sprintf("messages:dm:%s", m.ChannelID)
	}
	// Fetch last 50 messages to find the bot's last message.
	last50Messages, err := h.DB.GetLastNMessages(key, 50)
	if err != nil {
		h.Logger.Error("Failed to get engagement history from cache", err)
		userState.Mutex.Lock()
		userState.State = StateIdle
		userState.Timer.Stop()
		userState.Mutex.Unlock()
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

	// Reverse the history to be in chronological order.
	for i, j := 0, len(engagementHistory)-1; i < j; i, j = i+1, j-1 {
		engagementHistory[i], engagementHistory[j] = engagementHistory[j], engagementHistory[i]
	}

	decision, arg, err := h.LLMClient.GetEngagementDecision(s, m, engagementHistory)
	if err != nil {
		h.Logger.Error("Failed to check LLM engagement", err)
		userState.Mutex.Lock()
		userState.State = StateIdle
		userState.Timer.Stop()
		userState.Mutex.Unlock()
		return
	}

	h.Logger.Info(fmt.Sprintf("Engagement decision for message '%s': %s, arg: %s", m.Content, decision, arg))

	switch decision {
	case "REPLY":
		userState.Mutex.Lock()
		userState.State = StateStreaming

		ctx, cancel := context.WithCancel(context.Background())
		userState.CancelFunc = cancel
		userState.Mutex.Unlock()

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

		userState.Mutex.Lock()
		userState.InterruptedHistory = fullHistory
		userState.InterruptedContext = contextBlock
		userState.Mutex.Unlock()

		defer func() {
			userState.Mutex.Lock()
			userState.State = StateIdle
			userState.Timer.Stop()
			if ctx.Err() != context.Canceled {
				userState.InterruptedHistory = nil
				userState.InterruptedContext = ""
			}
			userState.Mutex.Unlock()
		}()

		responseMessage, err := h.LLMClient.StreamChatCompletion(ctx, s, m.Message, fullHistory, contextBlock)
		if err != nil {
			if err == context.Canceled {
				h.Logger.Info(fmt.Sprintf("Stream for user %s was interrupted.", m.Author.ID))
			} else {
				h.Logger.Error("Failed to stream LLM chat completion", err)
			}
			return
		}

		userState.Mutex.Lock()
		userState.MessageID = responseMessage.ID
		userState.ChannelID = responseMessage.ChannelID
		userState.Mutex.Unlock()

	case "REACT":
		userState.Mutex.Lock()
		userState.State = StateIdle
		userState.Timer.Stop()
		userState.Mutex.Unlock()
		if arg != "" {
			_ = s.MessageReactionAdd(m.ChannelID, m.ID, arg)
		}
	case "STOP":
		userState.Mutex.Lock()
		if userState.CancelFunc != nil {
			userState.CancelFunc()
		}
		userState.State = StateIdle
		userState.Timer.Stop()
		userState.Mutex.Unlock()
	case "CONTINUE":
		userState.Mutex.Lock()
		if userState.InterruptedHistory == nil {
			userState.State = StateIdle
			userState.Timer.Stop()
			userState.Mutex.Unlock()
			return
		}

		userState.State = StateStreaming
		ctx, cancel := context.WithCancel(context.Background())
		userState.CancelFunc = cancel
		userState.Mutex.Unlock()

		defer func() {
			userState.Mutex.Lock()
			userState.State = StateIdle
			userState.Timer.Stop()
			userState.InterruptedHistory = nil
			userState.InterruptedContext = ""
			userState.Mutex.Unlock()
		}()

		responseMessage, err := h.LLMClient.StreamChatCompletion(ctx, s, m.Message, userState.InterruptedHistory, userState.InterruptedContext)
		if err != nil {
			if err == context.Canceled {
				h.Logger.Info(fmt.Sprintf("Stream for user %s was interrupted.", m.Author.ID))
			} else {
				h.Logger.Error("Failed to stream LLM chat completion", err)
			}
			return
		}

		userState.Mutex.Lock()
		userState.MessageID = responseMessage.ID
		userState.ChannelID = responseMessage.ChannelID
		userState.Mutex.Unlock()

	case "IGNORE":
		userState.Mutex.Lock()
		userState.State = StateIdle
		userState.Timer.Stop()
		userState.Mutex.Unlock()
	}
}
