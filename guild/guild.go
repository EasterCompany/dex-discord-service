// Package guild defines data structures for guild-specific state and user audio streams.
package guild

import (
	"context"
	"sync"
	"time"
)

// TranscriptionEntry holds a single transcription record
type TranscriptionEntry struct {
	Duration      time.Duration
	Username      string
	Transcription string
	Timestamp     time.Time
	IsEvent       bool // True for speaking events, false for actual transcriptions
}

// GuildState holds the state for a single guild
type GuildState struct {
	Mutex                      sync.Mutex             `json:"-"`
	ActiveStreams              map[uint32]*UserStream `json:"-"`
	SSRCUserMap                map[uint32]string
	UnmappedSSRCs              map[uint32]bool      `json:"-"` // Track SSRCs we've received audio from but don't have user mappings for
	TranscriptionHistory       []TranscriptionEntry `json:"-"` // Full transcription log for the session
	ConnectionMessageID        string
	ConnectionChannelID        string
	ConnectionMessageChannelID string
	ConnectionStartTime        time.Time
	Ctx                        context.Context    `json:"-"` // Context for canceling the voice handler goroutine
	CancelFunc                 context.CancelFunc `json:"-"` // Function to cancel the context
}

// NewGuildState creates a new GuildState
func NewGuildState() *GuildState {
	ctx, cancel := context.WithCancel(context.Background())
	return &GuildState{
		ActiveStreams:              make(map[uint32]*UserStream),
		SSRCUserMap:                make(map[uint32]string),
		UnmappedSSRCs:              make(map[uint32]bool),
		TranscriptionHistory:       make([]TranscriptionEntry, 0),
		ConnectionMessageID:        "",
		ConnectionChannelID:        "",
		ConnectionMessageChannelID: "",
		ConnectionStartTime:        time.Time{},
		Ctx:                        ctx,
		CancelFunc:                 cancel,
	}
}
