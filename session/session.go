// eastercompany/dex-discord-interface/session/session.go
package session

import (
	"github.com/bwmarrin/discordgo"
)

// NewSession creates and configures a new Discord session.
func NewSession(token string) (*discordgo.Session, error) {
	// Create a new Discord session with the provided bot token.
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	// Specify the necessary intents.
	// Guilds is for basic server information.
	// GuildMessages is for receiving messages in channels.
	// GuildVoiceStates is for tracking who is joining/leaving/speaking in voice channels.
	s.Identify.Intents = discordgo.IntentGuilds | discordgo.IntentGuildMessages | discordgo.IntentGuildVoiceStates

	return s, nil
}
