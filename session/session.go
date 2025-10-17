package session

import (
	"github.com/bwmarrin/discordgo"
)

// NewSession creates a new Discord session
func NewSession(token string) (*discordgo.Session, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	return session, nil
}
