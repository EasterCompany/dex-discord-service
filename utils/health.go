package utils

import (
	"sync"
	"time"
)

// Health represents the health status of the service.
// This structure is UNIVERSAL across all services.
type Health struct {
	Status  string `json:"status"`
	Uptime  string `json:"uptime"`
	Message string `json:"message"`
}

var (
	startTime     = time.Now()
	currentHealth = Health{
		Status:  "STARTING",
		Uptime:  "0s",
		Message: "Service is initializing",
	}
	healthMu sync.RWMutex
)

// GetHealth returns the current health status of the service.
// This function is thread-safe and can be called from multiple goroutines.
func GetHealth() Health {
	healthMu.RLock()
	defer healthMu.RUnlock()

	// Update uptime every time health is checked
	health := currentHealth
	health.Uptime = time.Since(startTime).String()
	return health
}

// SetHealthStatus updates the health status of the service.
// This function is thread-safe and can be called from multiple goroutines.
func SetHealthStatus(status string, message string) {
	healthMu.Lock()
	defer healthMu.Unlock()

	currentHealth.Status = status
	currentHealth.Message = message
	currentHealth.Uptime = time.Since(startTime).String()
}
