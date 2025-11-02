package services

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// ServiceStatus represents the health status of a Dexter microservice
type ServiceStatus struct {
	Name         string    `json:"name"`
	Status       string    `json:"status"` // operational, degraded, offline
	Uptime       string    `json:"uptime"`
	Version      string    `json:"version"`
	LastCheck    time.Time `json:"last_check"`
	ResponseTime int64     `json:"response_time"` // milliseconds
	Endpoint     string    `json:"endpoint"`

	// Extended metrics (if provided by service)
	Metrics map[string]interface{} `json:"metrics,omitempty"`
}

// HealthChecker monitors the health of all Dexter microservices
type HealthChecker struct {
	mu            sync.RWMutex
	services      map[string]*ServiceStatus
	client        *http.Client
	checkInterval time.Duration
	stopChan      chan struct{}
}

// NewHealthChecker creates a new service health checker
func NewHealthChecker(checkInterval time.Duration) *HealthChecker {
	return &HealthChecker{
		services: make(map[string]*ServiceStatus),
		client: &http.Client{
			Timeout: 2 * time.Second, // Fast timeout for health checks
		},
		checkInterval: checkInterval,
		stopChan:      make(chan struct{}),
	}
}

// RegisterService adds a service to monitor
func (hc *HealthChecker) RegisterService(name, endpoint string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.services[name] = &ServiceStatus{
		Name:      name,
		Status:    "N/A",
		Endpoint:  endpoint,
		LastCheck: time.Now(),
	}

	log.Printf("[HEALTH] Registered service: %s (%s)", name, endpoint)
}

// Start begins monitoring all registered services
func (hc *HealthChecker) Start() {
	go hc.monitorLoop()
	log.Println("[HEALTH] Service health checker started")
}

// Stop halts the health checker
func (hc *HealthChecker) Stop() {
	close(hc.stopChan)
	log.Println("[HEALTH] Service health checker stopped")
}

// monitorLoop continuously checks service health
func (hc *HealthChecker) monitorLoop() {
	// Immediate first check
	hc.checkAllServices()

	ticker := time.NewTicker(hc.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hc.checkAllServices()
		case <-hc.stopChan:
			return
		}
	}
}

// checkAllServices polls all registered services
func (hc *HealthChecker) checkAllServices() {
	hc.mu.RLock()
	services := make(map[string]string) // name -> endpoint
	for name, status := range hc.services {
		services[name] = status.Endpoint
	}
	hc.mu.RUnlock()

	for name, endpoint := range services {
		go hc.checkService(name, endpoint)
	}
}

// checkService polls a single service status
func (hc *HealthChecker) checkService(name, endpoint string) {
	startTime := time.Now()

	resp, err := hc.client.Get(endpoint)
	responseTime := time.Since(startTime).Milliseconds()

	hc.mu.Lock()
	defer hc.mu.Unlock()

	status := hc.services[name]
	status.LastCheck = time.Now()
	status.ResponseTime = responseTime

	if err != nil {
		status.Status = "BAD"
		status.Uptime = ""
		status.Version = ""
		status.Metrics = nil
		log.Printf("[HEALTH] %s: OFFLINE (%v)", name, err)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		status.Status = "BAD"
		log.Printf("[HEALTH] %s: DEGRADED (HTTP %d)", name, resp.StatusCode)
		return
	}

	// Parse status response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		status.Status = "BAD"
		log.Printf("[HEALTH] %s: DEGRADED (failed to read response)", name)
		return
	}

	var serviceStatus map[string]interface{}
	if err := json.Unmarshal(body, &serviceStatus); err != nil {
		status.Status = "BAD"
		log.Printf("[HEALTH] %s: DEGRADED (invalid JSON)", name)
		return
	}

	// Extract standard fields
	if statusStr, ok := serviceStatus["status"].(string); ok {
		status.Status = statusStr
	} else {
		status.Status = "OK" // Default if responding
	}

	if uptime, ok := serviceStatus["uptime"].(string); ok {
		status.Uptime = uptime
	}

	if version, ok := serviceStatus["version"].(string); ok {
		status.Version = version
	}

	// Store all metrics
	if metrics, ok := serviceStatus["metrics"].(map[string]interface{}); ok {
		status.Metrics = metrics
	}

	log.Printf("[HEALTH] %s: %s (%dms)", name, status.Status, responseTime)
}

// GetServiceStatus returns the current status of a service
func (hc *HealthChecker) GetServiceStatus(name string) *ServiceStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	if status, ok := hc.services[name]; ok {
		// Return a copy
		statusCopy := *status
		return &statusCopy
	}
	return nil
}

// GetAllServices returns status of all services
func (hc *HealthChecker) GetAllServices() map[string]*ServiceStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	servicesCopy := make(map[string]*ServiceStatus)
	for name, status := range hc.services {
		statusCopy := *status
		servicesCopy[name] = &statusCopy
	}
	return servicesCopy
}

// GetStatusEmoji returns an emoji indicator for service status
func GetStatusEmoji(status string) string {
	switch status {
	case "OK":
		return "✅"
	case "BAD":
		return "❌"
	case "N/A":
		return "❓"
	default:
		return "❓"
	}
}
