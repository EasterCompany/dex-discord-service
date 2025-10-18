// eastercompany/dex-discord-interface/guild/guild.go
package guild

import (
	"sync"
	"time"
)

// GuildState holds the state for a single guild
type GuildState struct {
	Mutex         sync.Mutex `json:"-"`
	ActiveStreams map[uint32]*UserStream `json:"-"`
	SSRCUserMap   map[uint32]string
	ConnectionMessageID string
	ConnectionChannelID string
	ConnectionMessageChannelID string
	ConnectionStartTime time.Time
}

// NewGuildState creates a new GuildState
func NewGuildState() *GuildState {
	return &GuildState{
		ActiveStreams: make(map[uint32]*UserStream),
		SSRCUserMap:   make(map[uint32]string),
		ConnectionMessageID: "",
		ConnectionChannelID: "",
		ConnectionMessageChannelID: "",
		ConnectionStartTime: time.Time{},
	}
}
