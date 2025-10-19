package events

import (
	"sync"

	"github.com/EasterCompany/dex-discord-interface/guild"
)

// StateManager manages the state of all guilds.
type StateManager struct {
	guildStates        sync.Map
	addedMessagesCount int
	addedMessagesSize  int64
	mu                 sync.Mutex // Mutex to protect addedMessagesCount and addedMessagesSize
}

// NewStateManager creates a new state manager.
func NewStateManager() *StateManager {
	return &StateManager{}
}

// GetGuildState returns the state for a given guild.
func (sm *StateManager) GetGuildState(guildID string) (*guild.GuildState, bool) {
	value, ok := sm.guildStates.Load(guildID)
	if !ok {
		return nil, false
	}
	return value.(*guild.GuildState), true
}

// GetOrStoreGuildState returns the state for a given guild, creating it if it doesn't exist.
func (sm *StateManager) GetOrStoreGuildState(guildID string) *guild.GuildState {
	value, _ := sm.guildStates.LoadOrStore(guildID, guild.NewGuildState())
	return value.(*guild.GuildState)
}

// DeleteGuildState deletes the state for a given guild.
func (sm *StateManager) DeleteGuildState(guildID string) {
	sm.guildStates.Delete(guildID)
}

// SetAddedMessagesStats sets the statistics for messages added during initial sync.
func (sm *StateManager) SetAddedMessagesStats(count int, size int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.addedMessagesCount = count
	sm.addedMessagesSize = size
}

// GetAddedMessagesStats returns the statistics for messages added during initial sync.
func (sm *StateManager) GetAddedMessagesStats() (int, int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.addedMessagesCount, sm.addedMessagesSize
}
