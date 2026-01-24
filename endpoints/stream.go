package endpoints

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// StartStreamRequest represents the request to start a stream
type StartStreamRequest struct {
	ChannelID      string `json:"channel_id"`
	InitialContent string `json:"initial_content,omitempty"`
}

// StartStreamResponse represents the response when a stream starts
type StartStreamResponse struct {
	MessageID string `json:"message_id"`
}

// StreamRequest represents the request body for stream updates
type StreamRequest struct {
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"` // Required for Update/Complete
	Content   string `json:"content"`
}

type StreamSession struct {
	ChannelID      string
	MessageID      string // The ID of the FIRST message (Session Key)
	MessageIDs     []string
	CurrentContent string
	LastSentChunks []string // Content of each message last sent
	LastEdit       time.Time
	Done           bool
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
		ticker:  time.NewTicker(500 * time.Millisecond), // 500ms for smoother streaming
	}
	go streamManager.Run()
}

func chunkString(s string, chunkSize int) []string {
	if len(s) == 0 {
		return []string{""}
	}
	if len(s) <= chunkSize {
		return []string{s}
	}
	var chunks []string
	runes := []rune(s)

	currentStart := 0
	for currentStart < len(runes) {
		remaining := len(runes) - currentStart
		if remaining <= chunkSize {
			chunks = append(chunks, string(runes[currentStart:]))
			break
		}

		// Tentative split point at max size
		splitIdx := currentStart + chunkSize

		// Search backwards for a safe split point (newline or space)
		// Scan back up to 500 chars for newline
		foundSplit := false
		for i := splitIdx; i > currentStart && i > splitIdx-500; i-- {
			if runes[i] == '\n' {
				splitIdx = i + 1 // Include newline in the current chunk
				foundSplit = true
				break
			}
		}

		// If no newline, look for space (up to 200 chars back)
		if !foundSplit {
			for i := splitIdx; i > currentStart && i > splitIdx-200; i-- {
				if runes[i] == ' ' {
					splitIdx = i + 1 // Include space
					foundSplit = true
					break
				}
			}
		}

		// If still no split found, force split at limit
		chunks = append(chunks, string(runes[currentStart:splitIdx]))
		currentStart = splitIdx
	}
	return chunks
}

func (sm *StreamManager) Run() {
	for range sm.ticker.C {
		sm.mu.Lock()
		for id, session := range sm.streams {
			// Check if content has changed (simple length check or string comparison)
			// But we need to compare chunks to avoid re-editing unchanged parts.
			currentChunks := chunkString(session.CurrentContent, 2000)

			// 1. Expand messages if needed
			// Note: We do this first so we have IDs to edit
			for len(session.MessageIDs) < len(currentChunks) {
				idx := len(session.MessageIDs)
				newContent := currentChunks[idx]

				// If this is a new expansion, we send the message
				newMsg, err := discordSession.ChannelMessageSend(session.ChannelID, newContent)
				if err != nil {
					log.Printf("STREAM EXPANSION ERROR: Failed to send new message chunk %d: %v", idx, err)
					// Break expansion loop, will retry next tick
					break
				}
				session.MessageIDs = append(session.MessageIDs, newMsg.ID)
				// Append placeholder to LastSentChunks so length matches (will be updated below)
				session.LastSentChunks = append(session.LastSentChunks, "")
			}

			// 2. Update existing messages if their specific chunk changed
			for i, chunk := range currentChunks {
				if i >= len(session.MessageIDs) {
					// Should have been expanded above, but if it failed, stop here
					break
				}

				// Check if this chunk needs update
				needsUpdate := false
				if i >= len(session.LastSentChunks) {
					needsUpdate = true // New chunk
				} else if session.LastSentChunks[i] != chunk {
					needsUpdate = true // Changed chunk
				}

				if needsUpdate {
					msgID := session.MessageIDs[i]
					_, err := discordSession.ChannelMessageEdit(session.ChannelID, msgID, chunk)
					if err != nil {
						// Check for 404 (Unknown Message)
						if restErr, ok := err.(*discordgo.RESTError); ok && restErr.Response.StatusCode == 404 {
							log.Printf("STREAM RECOVERY: Message %s (chunk %d) deleted. Reposting...", msgID, i)
							newMsg, sendErr := discordSession.ChannelMessageSend(session.ChannelID, chunk)
							if sendErr == nil {
								session.MessageIDs[i] = newMsg.ID
								// If this was the first message, update the session key reference
								if i == 0 {
									session.MessageID = newMsg.ID
								}
								// Treat as success
								if i < len(session.LastSentChunks) {
									session.LastSentChunks[i] = chunk
								} else {
									session.LastSentChunks = append(session.LastSentChunks, chunk)
								}
							} else {
								log.Printf("STREAM RECOVERY FAILED: %v", sendErr)
							}
						} else {
							log.Printf("STREAM EDIT ERROR: Msg %s: %v", msgID, err)
						}
					} else {
						// Success
						if i < len(session.LastSentChunks) {
							session.LastSentChunks[i] = chunk
						} else {
							session.LastSentChunks = append(session.LastSentChunks, chunk)
						}
					}
				}
			}

			// Update timestamps
			session.LastEdit = time.Now()

			// Clean up if done
			// We consider "Synced" if LastSentChunks matches currentChunks
			synced := true
			if len(session.LastSentChunks) != len(currentChunks) {
				synced = false
			} else {
				for i, c := range currentChunks {
					if session.LastSentChunks[i] != c {
						synced = false
						break
					}
				}
			}

			if session.Done && synced {
				delete(sm.streams, id)
			}
		}
		sm.mu.Unlock()
	}
}

// StartStreamHandler creates a new message and initializes tracking
func StartStreamHandler(w http.ResponseWriter, r *http.Request) {
	var req StartStreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	initialContent := req.InitialContent
	if initialContent == "" {
		// Default typing emoji if no custom status provided
		initialContent = "<a:typing:1449387367315275786>"
	}

	msg, err := discordSession.ChannelMessageSend(req.ChannelID, initialContent)
	if err != nil {
		log.Printf("Error starting stream (sending message): %v, channel_id: %s", err, req.ChannelID)
		http.Error(w, "Failed to start stream", http.StatusInternalServerError)
		return
	}

	// Register with StreamManager immediately to ensure it's tracked
	streamManager.mu.Lock()
	streamManager.streams[msg.ID] = &StreamSession{
		ChannelID:      req.ChannelID,
		MessageID:      msg.ID,
		MessageIDs:     []string{msg.ID},
		CurrentContent: initialContent,
		LastSentChunks: []string{initialContent},
		LastEdit:       time.Now(),
		Done:           false,
	}
	streamManager.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(StartStreamResponse{
		MessageID: msg.ID,
	})
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
	// Look up by initial MessageID (Session Key)
	if session, ok := streamManager.streams[req.MessageID]; ok {
		if !session.Done {
			session.CurrentContent = req.Content
		}
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

	finalMessageID := req.MessageID

	streamManager.mu.Lock()
	if session, ok := streamManager.streams[req.MessageID]; ok {
		if req.Content != "" {
			session.CurrentContent = req.Content
		}
		session.Done = true
		finalMessageID = session.MessageID // Return the ID of the first message in the chain
	} else {
		// Fallback final edit
		if req.Content != "" && len(req.Content) <= 2000 {
			go func() {
				if _, err := discordSession.ChannelMessageEdit(req.ChannelID, req.MessageID, req.Content); err != nil {
					log.Printf("STREAM COMPLETE ERROR: Direct edit failed: %v", err)
				}
			}()
		}
	}
	streamManager.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message_id": finalMessageID,
	})
}
