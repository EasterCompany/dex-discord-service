package services

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// StatusServer provides HTTP status endpoint for this service
type StatusServer struct {
	startTime time.Time
	port      int
	version   string

	// Metrics
	messagesProcessed atomic.Uint64
	eventsProcessed   atomic.Uint64
	snapshotsCaptured atomic.Uint64
	voiceConnections  atomic.Uint64
}

// NewStatusServer creates a new status server
func NewStatusServer(port int, version string) *StatusServer {
	return &StatusServer{
		startTime: time.Now(),
		port:      port,
		version:   version,
	}
}

// Start begins the HTTP status server
func (ss *StatusServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", ss.handleStatus)
	mux.HandleFunc("/health", ss.handleHealth)
	mux.HandleFunc("/service", ss.handleService) // New handler for service information

	addr := fmt.Sprintf(":%d", ss.port)
	log.Printf("[STATUS] Starting status server on 0.0.0.0:%d", ss.port)

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
		"status":    "OK",
		"version":   ss.version,
		"uptime":    int(uptime.Seconds()),
		"timestamp": time.Now().Unix(),
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
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Printf("[STATUS] Error encoding status: %v", err)
	}
}

// handleService returns service information in standardized format
func (ss *StatusServer) handleService(w http.ResponseWriter, r *http.Request) {
	// Support ?format=version for simple version string
	if r.URL.Query().Get("format") == "version" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ss.version))
		return
	}

	// Parse version string: major.minor.patch.branch.commit.buildDate.arch.buildHash
	versionParts := strings.Split(ss.version, ".")
	versionObj := map[string]string{
		"major":      "0",
		"minor":      "0",
		"patch":      "0",
		"branch":     "",
		"commit":     "",
		"build_date": "",
		"arch":       "",
		"build_hash": "",
	}
	if len(versionParts) >= 8 {
		versionObj["major"] = versionParts[0]
		versionObj["minor"] = versionParts[1]
		versionObj["patch"] = versionParts[2]
		versionObj["branch"] = versionParts[3]
		versionObj["commit"] = versionParts[4]
		versionObj["build_date"] = versionParts[5]
		versionObj["arch"] = versionParts[6]
		versionObj["build_hash"] = versionParts[7]
	}

	uptime := time.Since(ss.startTime)

	// Get memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Standard service response format (matches dex-event-service)
	response := map[string]interface{}{
		"version": map[string]interface{}{
			"str": ss.version,
			"obj": versionObj,
		},
		"health": map[string]interface{}{
			"status":  "OK",
			"uptime":  uptime.String(),
			"message": "Service is running normally",
		},
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
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[STATUS] Error encoding service response: %v", err)
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
