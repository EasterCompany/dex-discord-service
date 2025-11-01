package handlers

import (
	"log"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// StatusManager manages the bot's presence and status updates.
type StatusManager struct {
	session       *discordgo.Session
	mu            sync.RWMutex
	currentStatus string
	idleTimer     *time.Timer
}

// NewStatusManager creates a new StatusManager.
func NewStatusManager(session *discordgo.Session) *StatusManager {
	return &StatusManager{
		session: session,
	}
}

// GetStatus returns the current status of the bot.
func (sm *StatusManager) GetStatus() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.currentStatus
}

// SetSleeping sets the bot's status to "Sleeping...".
func (sm *StatusManager) SetSleeping() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.setStatus("online", "Sleeping...", discordgo.ActivityTypeGame)
	sm.currentStatus = "Sleeping"
	if sm.idleTimer != nil {
		sm.idleTimer.Stop()
	}
}

// SetIdle sets the bot's status to "Idle" for a limited time.
func (sm *StatusManager) SetIdle() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.setStatus("online", "Idle", discordgo.ActivityTypeGame)
	sm.currentStatus = "Idle"

	if sm.idleTimer != nil {
		sm.idleTimer.Stop()
	}
	sm.idleTimer = time.AfterFunc(5*time.Minute, func() {
		sm.SetSleeping()
	})
}

// SetThinking sets the bot's status to "Thinking...".
func (sm *StatusManager) SetThinking() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.setStatus("online", "Thinking...", discordgo.ActivityTypeGame)
	sm.currentStatus = "Thinking"
	if sm.idleTimer != nil {
		sm.idleTimer.Stop()
	}
}

// setStatus is a private helper to update the bot's status.
func (sm *StatusManager) setStatus(status, message string, activityType discordgo.ActivityType) {
	err := sm.session.UpdateStatusComplex(discordgo.UpdateStatusData{
		Status: status,
		Activities: []*discordgo.Activity{
			{
				Name: message,
				Type: activityType,
			},
		},
	})
	if err != nil {
		log.Printf("Warning: Failed to set status: %v", err)
	}
}
