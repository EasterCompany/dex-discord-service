// eastercompany/dex-discord-interface/guild/userstream.go
package guild

import (
	"context"
	"io"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
)

// UserStream holds the state for a single user's audio stream.
type UserStream struct {
	Writer     io.Closer
	OggWriter  *oggwriter.OggWriter
	CancelFunc context.CancelFunc
	LastPacket time.Time
	Message    *discordgo.Message
	User       *discordgo.User
	StartTime  time.Time
	GuildID    string
	ChannelID  string
}
