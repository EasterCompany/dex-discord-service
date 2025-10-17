// eastercompany/dex-discord-interface/main.go
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/events"
	"github.com/EasterCompany/dex-discord-interface/session"
	"github.com/EasterCompany/dex-discord-interface/stt"
)

func main() {
	// Load configuration from ~/Dexter/config.json
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Fatal error loading config: %v", err)
	}

	// Initialize the Google Speech-to-Text service
	if err := stt.Initialize(); err != nil {
		log.Fatalf("Fatal error initializing STT service: %v", err)
	}
	defer stt.Close()

	// Create a new Discord session using the token from the config
	s, err := session.NewSession(cfg.System.Discord.Token)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	// Add event handlers
	s.AddHandler(events.MessageCreate)
	s.AddHandler(events.SpeakingUpdate) // Use the correct handler for speaking updates

	// Open a websocket connection to Discord and begin listening
	err = s.Open()
	if err != nil {
		log.Fatalf("Error opening connection: %v", err)
	}

	// Wait here until CTRL-C or other term signal is received
	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly close down the Discord session
	s.Close()
	fmt.Println("\nBot shutting down.")
}
