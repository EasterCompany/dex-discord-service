package guild

import (
	"bytes"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
)

// UserStream holds the state for a single user's audio stream.
type UserStream struct {
	VoiceChannelID string
	OggWriter      *oggwriter.OggWriter
	Buffer         *bytes.Buffer
	LastPacket     time.Time
	User           *discordgo.User
	StartTime      time.Time
	Filename       string
}
