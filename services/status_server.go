package services

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"
)

// StatusServer provides HTTP status endpoint for this service
type StatusServer struct {
	startTime     time.Time
	port          int
	version       string
	healthChecker *HealthChecker

	// Metrics
	messagesProcessed atomic.Uint64
	eventsProcessed   atomic.Uint64
	snapshotsCaptured atomic.Uint64
	voiceConnections  atomic.Uint64
}

// NewStatusServer creates a new status server
func NewStatusServer(port int, version string, healthChecker *HealthChecker) *StatusServer {
	return &StatusServer{
		startTime:     time.Now(),
		port:          port,
		version:       version,
		healthChecker: healthChecker,
	}
}

// Start begins the HTTP status server
func (ss *StatusServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", ss.handleStatus)
	mux.HandleFunc("/health", ss.handleHealth)
	mux.HandleFunc("/services", ss.handleServices)

	addr := fmt.Sprintf("127.0.0.1:%d", ss.port)
	log.Printf("[STATUS] Starting status server on http://%s", addr)

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("[STATUS] Server error: %v", err)
		}
	}()

	return nil
}

// handleStatus returns detailed service status
func (ss *StatusServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(ss.startTime)

	// Get memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	status := map[string]interface{}{
		"service":   "dex-discord-service",
		"status":    "operational",
		"version":   ss.version,
		"uptime":    uptime.String(),
		"timestamp": time.Now().Format(time.RFC3339),
		"metrics": map[string]interface{}{
			"messages_processed": ss.messagesProcessed.Load(),
			"events_processed":   ss.eventsProcessed.Load(),
			"snapshots_captured": ss.snapshotsCaptured.Load(),
			"voice_connections":  ss.voiceConnections.Load(),
			"goroutines":         runtime.NumGoroutine(),
			"memory_alloc_mb":    float64(m.Alloc) / 1024 / 1024,
			"memory_total_mb":    float64(m.TotalAlloc) / 1024 / 1024,
			"memory_sys_mb":      float64(m.Sys) / 1024 / 1024,
			"gc_runs":            m.NumGC,
		},
		"services": ss.healthChecker.GetAllServices(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Printf("[STATUS] Error encoding status: %v", err)
	}
}

// handleHealth returns simple health check (for load balancers)
func (ss *StatusServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	}); err != nil {
		log.Printf("[STATUS] Error encoding health: %v", err)
	}
}

// handleServices returns status of all monitored services
func (ss *StatusServer) handleServices(w http.ResponseWriter, r *http.Request) {
	services := ss.healthChecker.GetAllServices()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"services": services,
		"count":    len(services),
	}); err != nil {
		log.Printf("[STATUS] Error encoding services: %v", err)
	}
}

// Metric incrementers (called from handlers)
func (ss *StatusServer) IncrementMessages() {
	ss.messagesProcessed.Add(1)
}

func (ss *StatusServer) IncrementEvents() {
	ss.eventsProcessed.Add(1)
}

func (ss *StatusServer) IncrementSnapshots() {
	ss.snapshotsCaptured.Add(1)
}

func (ss *StatusServer) IncrementVoiceConnections() {
	ss.voiceConnections.Add(1)
}
