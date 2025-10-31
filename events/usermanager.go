package events

import (
	"context"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// UserState represents the state of a user's interaction with the bot.
type UserState int

const (
	StateIdle UserState = iota
	StatePending
	StateStreaming
)

// UserInteractionState holds the full state for a user.
type UserInteractionState struct {
	State              UserState
	Timer              *time.Ticker
	CancelFunc         context.CancelFunc
	MessageID          string
	ChannelID          string
	Mutex              sync.Mutex
	InterruptedHistory []*discordgo.Message
	InterruptedContext string
}

// UserManager manages the interaction state for all users.
type UserManager struct {
	users sync.Map // map[string]*UserInteractionState
}

// NewUserManager creates a new UserManager.
func NewUserManager() *UserManager {
	return &UserManager{}
}

// GetOrCreateUserState retrieves the state for a user, creating it if it doesn't exist.
func (um *UserManager) GetOrCreateUserState(userID string) *UserInteractionState {
	state, _ := um.users.LoadOrStore(userID, &UserInteractionState{})
	return state.(*UserInteractionState)
}

// TransitionToPending cancels any ongoing operation, sets the state to Pending,
// and starts the typing indicator.
func (s *UserInteractionState) TransitionToPending(session *discordgo.Session, channelID string) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	// Cancel previous state if not idle
	if s.State != StateIdle {
		if s.CancelFunc != nil {
			s.CancelFunc()
		}
		if s.Timer != nil {
			s.Timer.Stop()
		}
		// If it was streaming, delete the partial message
		if s.State == StateStreaming && s.MessageID != "" {
			_ = session.ChannelMessageDelete(s.ChannelID, s.MessageID)
		}
	}

	s.State = StatePending

	// Start a ticker to keep the typing indicator alive
	ticker := time.NewTicker(8 * time.Second)
	s.Timer = ticker
	go func() {
		// Initial typing indicator
		_ = session.ChannelTyping(channelID)
		for range ticker.C {
			_ = session.ChannelTyping(channelID)
		}
	}()
}

// TransitionToStreaming sets the state to Streaming and returns a new context.
func (s *UserInteractionState) TransitionToStreaming() context.Context {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	s.State = StateStreaming
	ctx, cancel := context.WithCancel(context.Background())
	s.CancelFunc = cancel
	return ctx
}

// TransitionToIdle resets the state to Idle and cleans up resources.
func (s *UserInteractionState) TransitionToIdle() {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	if s.Timer != nil {
		s.Timer.Stop()
		s.Timer = nil
	}
	s.State = StateIdle
	s.CancelFunc = nil
	s.MessageID = ""
	s.ChannelID = ""
}

// SaveInterruptedState saves the history and context for a potential continuation.
func (s *UserInteractionState) SaveInterruptedState(history []*discordgo.Message, context string) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	s.InterruptedHistory = history
	s.InterruptedContext = context
}

// ClearInterruptedState clears the saved history and context.
func (s *UserInteractionState) ClearInterruptedState() {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	s.InterruptedHistory = nil
	s.InterruptedContext = ""
}
