package endpoints

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// StreamRequest represents the request body for stream updates
type StreamRequest struct {
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"` // Required for Update/Complete
	Content   string `json:"content"`
}

type StreamSession struct {
	ChannelID       string
	MessageID       string
	CurrentContent  string
	LastSentContent string
	LastEdit        time.Time
	Done            bool
}

type StreamManager struct {
	streams map[string]*StreamSession
	mu      sync.Mutex
	ticker  *time.Ticker
}

var streamManager *StreamManager

func InitStreamManager() {
	streamManager = &StreamManager{
		streams: make(map[string]*StreamSession),
		ticker:  time.NewTicker(1000 * time.Millisecond), // 1 second throttling
	}
	go streamManager.Run()
}

func (sm *StreamManager) Run() {
	for range sm.ticker.C {
		sm.mu.Lock()
		for id, session := range sm.streams {
			// If content has changed and enough time has passed (or if it's the final update)
			if session.CurrentContent != session.LastSentContent {
				// Discord rate limit bucket for message edits is roughly 1/1s per message
				// but global rate limits also apply. 1s is safe.
				// We prioritize the edit.

				// Perform Edit
				_, err := discordSession.ChannelMessageEdit(session.ChannelID, session.MessageID, session.CurrentContent)
				if err != nil {
					log.Printf("STREAM ERROR: Failed to edit message %s: %v", session.MessageID, err)
					// If error is 404 (message deleted), stop streaming
					if restErr, ok := err.(*discordgo.RESTError); ok && restErr.Response.StatusCode == 404 {
						delete(sm.streams, id)
						continue
					}
				} else {
					session.LastSentContent = session.CurrentContent
					session.LastEdit = time.Now()
				}
			}

			// Clean up if done and synchronized
			if session.Done && session.CurrentContent == session.LastSentContent {
				delete(sm.streams, id)
			}
		}
		sm.mu.Unlock()
	}
}

// StartStreamHandler creates a new message and initializes tracking
func StartStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.ChannelID == "" {
		http.Error(w, "channel_id required", http.StatusBadRequest)
		return
	}

	// Send initial message
	initialContent := "..."
	if req.Content != "" {
		initialContent = req.Content
	}

	msg, err := discordSession.ChannelMessageSend(req.ChannelID, initialContent)
	if err != nil {
		log.Printf("STREAM START ERROR: %v", err)
		http.Error(w, "Failed to send initial message", http.StatusInternalServerError)
		return
	}

	streamManager.mu.Lock()
	streamManager.streams[msg.ID] = &StreamSession{
		ChannelID:       req.ChannelID,
		MessageID:       msg.ID,
		CurrentContent:  initialContent,
		LastSentContent: initialContent,
		LastEdit:        time.Now(),
		Done:            false,
	}
	streamManager.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"message_id": msg.ID,
		"channel_id": req.ChannelID,
	}); err != nil {
		log.Printf("STREAM START ERROR: Failed to encode response: %v", err)
	}
}

// UpdateStreamHandler updates the target content for a stream
func UpdateStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.MessageID == "" {
		http.Error(w, "message_id required", http.StatusBadRequest)
		return
	}

	streamManager.mu.Lock()
	if session, ok := streamManager.streams[req.MessageID]; ok {
		session.CurrentContent = req.Content
	} else {
		// Fallback: Direct Edit
		go func() {
			if _, err := discordSession.ChannelMessageEdit(req.ChannelID, req.MessageID, req.Content); err != nil {
				log.Printf("STREAM UPDATE ERROR: Direct edit failed: %v", err)
			}
		}()
	}
	streamManager.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

// CompleteStreamHandler marks a stream as done (will flush final content)
func CompleteStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.MessageID == "" {
		http.Error(w, "message_id required", http.StatusBadRequest)
		return
	}

	streamManager.mu.Lock()
	if session, ok := streamManager.streams[req.MessageID]; ok {
		if req.Content != "" {
			session.CurrentContent = req.Content
		}
		session.Done = true
	} else {
		// Fallback final edit
		if req.Content != "" {
			go func() {
				if _, err := discordSession.ChannelMessageEdit(req.ChannelID, req.MessageID, req.Content); err != nil {
					log.Printf("STREAM COMPLETE ERROR: Direct edit failed: %v", err)
				}
			}()
		}
	}
	streamManager.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}
