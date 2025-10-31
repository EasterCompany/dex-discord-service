package handlers

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

// ConnectHandler logs when the bot successfully connects to the Discord gateway.
func ConnectHandler() func(*discordgo.Session, *discordgo.Connect) {
	return func(s *discordgo.Session, c *discordgo.Connect) {
		log.Println("Gateway connection established!")
	}
}

// DisconnectHandler logs when the bot disconnects from the Discord gateway.
func DisconnectHandler() func(*discordgo.Session, *discordgo.Disconnect) {
	return func(s *discordgo.Session, d *discordgo.Disconnect) {
		log.Println("Gateway connection lost! Bot has disconnected.")
	}
}
