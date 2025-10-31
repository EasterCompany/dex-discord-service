package handlers

import (
	"fmt"
	"reflect"
	"time"

	"github.com/EasterCompany/dex-discord-interface/dashboard"
	"github.com/bwmarrin/discordgo"
)

// GenericEventHandler logs any Discord event to the events dashboard.
func GenericEventHandler(d *dashboard.EventsDashboard) func(s *discordgo.Session, i interface{}) {
	return func(s *discordgo.Session, i interface{}) {
		timestamp := time.Now().Format("15:04:05")
		var logEntry string

		// Use a type switch to handle different event types
		switch e := i.(type) {
		case *discordgo.MessageCreate, *discordgo.PresenceUpdate, *discordgo.TypingStart, *discordgo.Event:
			// Ignore noisy or already handled events
			return

		case *discordgo.MessageUpdate:
			channel, err := s.Channel(e.ChannelID)
			channelName := e.ChannelID
			if err == nil {
				channelName = channel.Name
			}
			author := "someone"
			if e.Author != nil {
				author = e.Author.Username
			}
			logEntry = fmt.Sprintf("[%s] @%s edited a message in #%s", timestamp, author, channelName)

		case *discordgo.MessageReactionAdd:
			logEntry = fmt.Sprintf("[%s] Reaction %s added by @%s", timestamp, e.Emoji.Name, e.Member.User.Username)

		case *discordgo.MessageReactionRemove:
			user, err := s.User(e.UserID)
			username := e.UserID
			if err == nil {
				username = user.Username
			}
			logEntry = fmt.Sprintf("[%s] Reaction %s removed by @%s", timestamp, e.Emoji.Name, username)

		case *discordgo.GuildMemberAdd:
			logEntry = fmt.Sprintf("[%s] @%s joined the server", timestamp, e.User.Username)

		case *discordgo.GuildMemberRemove:
			logEntry = fmt.Sprintf("[%s] @%s left the server", timestamp, e.User.Username)

		case *discordgo.ChannelCreate:
			logEntry = fmt.Sprintf("[%s] Channel #%s created", timestamp, e.Name)

		case *discordgo.ChannelDelete:
			logEntry = fmt.Sprintf("[%s] Channel #%s deleted", timestamp, e.Name)

		case *discordgo.VoiceStateUpdate:
			// Ignore mute/deafen and other minor state changes to reduce noise
			if e.BeforeUpdate != nil && e.ChannelID == e.BeforeUpdate.ChannelID {
				return
			}

			username := e.Member.User.Username
			if e.ChannelID == "" {
				// User left a voice channel
				channel, err := s.Channel(e.BeforeUpdate.ChannelID)
				channelName := e.BeforeUpdate.ChannelID
				if err == nil {
					channelName = channel.Name
				}
				logEntry = fmt.Sprintf("[%s] @%s left voice #%s", timestamp, username, channelName)
			} else if e.BeforeUpdate == nil || e.BeforeUpdate.ChannelID == "" {
				// User joined a voice channel
				channel, err := s.Channel(e.ChannelID)
				channelName := e.ChannelID
				if err == nil {
					channelName = channel.Name
				}
				logEntry = fmt.Sprintf("[%s] @%s joined voice #%s", timestamp, username, channelName)
			} else {
				// User moved between voice channels
				oldChannel, _ := s.Channel(e.BeforeUpdate.ChannelID)
				newChannel, _ := s.Channel(e.ChannelID)
				logEntry = fmt.Sprintf("[%s] @%s moved from #%s to #%s", timestamp, username, oldChannel.Name, newChannel.Name)
			}

		default:
			// Fallback for any other event type
			eventType := reflect.TypeOf(i).Elem().Name()
			logEntry = fmt.Sprintf("[%s] Event: %s", timestamp, eventType)
		}

		if logEntry != "" {
			d.AddEvent(logEntry)
		}
	}
}
