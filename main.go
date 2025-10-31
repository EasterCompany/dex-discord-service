package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/dashboard"
	"github.com/EasterCompany/dex-discord-interface/handlers"
	"github.com/bwmarrin/discordgo"
)

var dashboardManager *dashboard.Manager

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("Loaded config for server: %s\n", cfg.ServerID)

	// Create Discord session
	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		log.Fatalf("Failed to create Discord session: %v", err)
	}

	// Set intents
	session.Identify.Intents = discordgo.IntentsAll

	// Add ready handler to initialize dashboards after state is populated
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Println("Discord session is ready.")

		// Clean log channel before creating new dashboards
		if err := dashboard.CleanLogChannel(s, cfg.LogChannelID); err != nil {
			log.Printf("Warning: Failed to clean log channel: %v", err)
		}

		// Initialize dashboard manager
		dashboardManager = dashboard.NewManager(s, cfg.LogChannelID, cfg.ServerID)

		// Initialize all dashboards
		if err := dashboardManager.Init(); err != nil {
			log.Fatalf("Failed to initialize dashboards: %v", err)
		}

		// Force initial update to populate server info immediately
		if err := dashboardManager.Server.ForceUpdate(); err != nil {
			log.Printf("Warning: Failed to update server dashboard: %v", err)
		}

		fmt.Println("Dashboards initialized!")

		// Setup event handlers
		s.AddHandler(handlers.MessageCreateHandler(dashboardManager.Messages))
		log.Println("Message handler registered.")
	})

	// Open connection
	if err := session.Open(); err != nil {
		log.Fatalf("Failed to open Discord connection: %v", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			log.Printf("Error closing Discord session: %v", err)
		}
	}()

	fmt.Println("Connected to Discord! Waiting for ready event...")

	// TODO: Initialize Redis cache

	fmt.Println("Dexter Discord Interface running...")

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	fmt.Println("Shutting down...")

	// Cleanup dashboards
	if dashboardManager != nil {
		if err := dashboardManager.Shutdown(); err != nil {
			log.Printf("Error during dashboard shutdown: %v", err)
		}
	}

	fmt.Println("Shutdown complete.")
}
