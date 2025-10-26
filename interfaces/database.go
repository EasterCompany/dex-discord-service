// Package interfaces defines interfaces for various application components.
package interfaces

import (
	"github.com/EasterCompany/dex-discord-interface/guild"
	"github.com/bwmarrin/discordgo"
)

// Database is the interface for the database module
type Database interface {
	SaveMessage(guildID, channelID string, m *discordgo.Message) error
	SaveMessageHistory(guildID, channelID string, messages []*discordgo.Message) error
	LogTranscription(guildID, channelID, user, transcription string) error
	SaveGuildState(guildID string, state *guild.GuildState) error
	LoadGuildState(guildID string) (*guild.GuildState, error)
	GetAllGuildIDs() ([]string, error)
}
