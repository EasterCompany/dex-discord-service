package handlers

import (
	"log"
	"sync"
	"time"

	"github.com/EasterCompany/dex-discord-interface/dashboard"
)

// HealthMonitor tracks system health and triggers auto-connect to voice
type HealthMonitor struct {
	mu sync.RWMutex

	// System status flags
	discordReady    bool
	dashboardsReady bool
	redisReady      bool

	// Tracking
	greenSince      *time.Time
	autoConnectDone bool

	// Configuration
	guildID           string
	defaultChannelID  string
	stabilityDuration time.Duration

	// Components
	voiceManager    *VoiceConnectionManager
	statusManager   *StatusManager
	logsDashboard   *dashboard.LogsDashboard
	eventsDashboard *dashboard.EventsDashboard
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(
	guildID, defaultChannelID string,
	voiceManager *VoiceConnectionManager,
	statusManager *StatusManager,
	logsDashboard *dashboard.LogsDashboard,
	eventsDashboard *dashboard.EventsDashboard,
) *HealthMonitor {
	return &HealthMonitor{
		guildID:           guildID,
		defaultChannelID:  defaultChannelID,
		stabilityDuration: 60 * time.Second, // 1 minute
		voiceManager:      voiceManager,
		statusManager:     statusManager,
		logsDashboard:     logsDashboard,
		eventsDashboard:   eventsDashboard,
	}
}

// SetDiscordReady marks Discord as ready
func (h *HealthMonitor) SetDiscordReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.discordReady = ready
	h.checkAndUpdateGreenStatus()
}

// SetDashboardsReady marks dashboards as ready
func (h *HealthMonitor) SetDashboardsReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dashboardsReady = ready
	h.checkAndUpdateGreenStatus()
}

// SetRedisReady marks Redis as ready
func (h *HealthMonitor) SetRedisReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.redisReady = ready
	h.checkAndUpdateGreenStatus()
}

// checkAndUpdateGreenStatus checks if all systems are green and updates tracking
// Must be called with lock held
func (h *HealthMonitor) checkAndUpdateGreenStatus() {
	allGreen := h.discordReady && h.dashboardsReady && h.redisReady

	if allGreen {
		if h.greenSince == nil {
			// First time all green
			now := time.Now()
			h.greenSince = &now
			log.Println("[HEALTH] All systems operational")
		}
	} else {
		// Not all green anymore
		if h.greenSince != nil {
			log.Println("[HEALTH] System instability detected")
			h.greenSince = nil
		}
	}
}

// Start begins the health monitoring loop
func (h *HealthMonitor) Start() {
	go h.monitorLoop()
}

// monitorLoop checks health status periodically
func (h *HealthMonitor) monitorLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	log.Println("[HEALTH] Health monitor started")

	for range ticker.C {
		h.mu.Lock()

		// Skip if already auto-connected or no default channel configured
		if h.autoConnectDone || h.defaultChannelID == "" {
			h.mu.Unlock()
			continue
		}

		// Check if we've been green for long enough
		if h.greenSince != nil {
			greenDuration := time.Since(*h.greenSince)

			if greenDuration >= h.stabilityDuration {
				// Time to auto-connect!
				log.Printf("[HEALTH] System stable for %s - auto-connecting to voice channel\n", greenDuration.Round(time.Second))
				h.autoConnectDone = true

				// Unlock before calling voiceManager to avoid deadlock
				guildID := h.guildID
				channelID := h.defaultChannelID
				h.mu.Unlock()

				// Wake the bot up before auto-connecting
				if h.statusManager != nil {
					h.statusManager.SetIdle()
				}

				// Perform auto-connect
				h.performAutoConnect(guildID, channelID)
				continue
			}
		}

		h.mu.Unlock()
	}
}

// performAutoConnect attempts to join the default voice channel
func (h *HealthMonitor) performAutoConnect(guildID, channelID string) {
	log.Printf("[HEALTH] Auto-connecting to voice channel\n")
	h.eventsDashboard.AddEvent("[AUTO-JOIN] Connecting to default voice channel")

	err := h.voiceManager.JoinVoiceChannel(guildID, channelID)
	if err != nil {
		log.Printf("[HEALTH] Auto-connect failed: %v\n", err)
		h.eventsDashboard.AddEvent("[AUTO-JOIN] Failed - will retry")

		// Reset auto-connect flag so we can retry
		h.mu.Lock()
		h.autoConnectDone = false
		h.greenSince = nil // Reset the timer
		h.mu.Unlock()
	} else {
		log.Println("[HEALTH] Auto-connect successful")
		h.eventsDashboard.AddEvent("[AUTO-JOIN] Connected successfully")
	}
}

// GetStatus returns the current health status
func (h *HealthMonitor) GetStatus() (allGreen bool, greenDuration time.Duration) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	allGreen = h.discordReady && h.dashboardsReady && h.redisReady
	if h.greenSince != nil {
		greenDuration = time.Since(*h.greenSince)
	}
	return
}
