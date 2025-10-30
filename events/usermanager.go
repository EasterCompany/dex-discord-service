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
