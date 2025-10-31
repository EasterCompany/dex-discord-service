package dashboard

import (
	"sync"

	"github.com/bwmarrin/discordgo"
)

// VoiceState represents the current state of voice channels
type VoiceState struct {
	mu       sync.RWMutex
	channels map[string]map[string]string // channelID -> userID -> username
}

// NewVoiceState creates a new VoiceState manager
func NewVoiceState() *VoiceState {
	return &VoiceState{
		channels: make(map[string]map[string]string),
	}
}

// Update handles a VoiceStateUpdate event
func (vs *VoiceState) Update(update *discordgo.VoiceStateUpdate) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// User left a channel
	if update.ChannelID == "" {
		if update.BeforeUpdate != nil {
			if users, ok := vs.channels[update.BeforeUpdate.ChannelID]; ok {
				delete(users, update.UserID)
				if len(users) == 0 {
					delete(vs.channels, update.BeforeUpdate.ChannelID)
				}
			}
		}
		return
	}

	// User joined or moved to a new channel
	// First, remove from old channel if necessary
	if update.BeforeUpdate != nil && update.BeforeUpdate.ChannelID != "" {
		if users, ok := vs.channels[update.BeforeUpdate.ChannelID]; ok {
			delete(users, update.UserID)
			if len(users) == 0 {
				delete(vs.channels, update.BeforeUpdate.ChannelID)
			}
		}
	}

	// Add to new channel
	if _, ok := vs.channels[update.ChannelID]; !ok {
		vs.channels[update.ChannelID] = make(map[string]string)
	}
	vs.channels[update.ChannelID][update.UserID] = update.Member.User.Username
}

// GetChannels returns a copy of the current voice channel state
func (vs *VoiceState) GetChannels() map[string]map[string]string {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	// Return a deep copy to avoid race conditions
	channelsCopy := make(map[string]map[string]string)
	for channelID, users := range vs.channels {
		usersCopy := make(map[string]string)
		for userID, username := range users {
			usersCopy[userID] = username
		}
		channelsCopy[channelID] = usersCopy
	}
	return channelsCopy
}
