// eastercompany/dex-discord-interface/health/health.go
package health

import (
	"github.com/bwmarrin/discordgo"
)

// CheckDiscordConnection checks the status of the Discord connection
func CheckDiscordConnection(s *discordgo.Session) error {
	// The Open function already establishes the connection, so we just need to check the status.
	if s.DataReady {
		return nil
	}
	return s.Open()
}
