package events

import (
	"sync"

	"github.com/EasterCompany/dex-discord-interface/guild"
)

// StateManager manages the state of all guilds.
type StateManager struct {
	guildStates sync.Map
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
