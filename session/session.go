// Package session manages the Discord session.
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

	// Specify all necessary intents for admin-level access and comprehensive monitoring.
	// This includes all guild-related events, voice states, messages, reactions, and more.
	s.Identify.Intents = discordgo.IntentGuilds |
		discordgo.IntentGuildMembers |
		discordgo.IntentGuildModeration |
		discordgo.IntentGuildEmojis |
		discordgo.IntentGuildIntegrations |
		discordgo.IntentGuildWebhooks |
		discordgo.IntentGuildInvites |
		discordgo.IntentGuildVoiceStates |
		discordgo.IntentGuildPresences |
		discordgo.IntentGuildMessages |
		discordgo.IntentGuildMessageReactions |
		discordgo.IntentGuildMessageTyping |
		discordgo.IntentDirectMessages |
		discordgo.IntentDirectMessageReactions |
		discordgo.IntentDirectMessageTyping |
		discordgo.IntentMessageContent |
		discordgo.IntentGuildScheduledEvents |
		discordgo.IntentAutoModerationConfiguration |
		discordgo.IntentAutoModerationExecution |
		discordgo.IntentGuildMessagePolls

	return s, nil
}
