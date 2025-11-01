package handlers

import (
	"log"
	"sync"
	"time"

	"github.com/EasterCompany/dex-discord-service/dashboard"
)

// HealthMonitor tracks system health
type HealthMonitor struct {
	mu sync.RWMutex

	// System status flags
	discordReady    bool
	dashboardsReady bool
	redisReady      bool

	// Tracking
	greenSince *time.Time

	// Configuration
	guildID string

	// Components
	statusManager   *StatusManager
	logsDashboard   *dashboard.LogsDashboard
	eventsDashboard *dashboard.EventsDashboard
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(
	guildID string,
	statusManager *StatusManager,
	logsDashboard *dashboard.LogsDashboard,
	eventsDashboard *dashboard.EventsDashboard,
) *HealthMonitor {
	return &HealthMonitor{
		guildID:         guildID,
		statusManager:   statusManager,
		logsDashboard:   logsDashboard,
		eventsDashboard: eventsDashboard,
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
		// Health monitor just tracks system health
		// No auto-connect functionality
		h.mu.RLock()
		_ = h.discordReady && h.dashboardsReady && h.redisReady
		h.mu.RUnlock()

		// System health is tracked, continue monitoring
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
