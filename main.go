package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/dashboard"
	"github.com/bwmarrin/discordgo"
)

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

	// Open connection
	if err := session.Open(); err != nil {
		log.Fatalf("Failed to open Discord connection: %v", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			log.Printf("Error closing Discord session: %v", err)
		}
	}()

	fmt.Println("Connected to Discord!")

	// Clean log channel before creating new dashboards
	if err := dashboard.CleanLogChannel(session, cfg.LogChannelID); err != nil {
		log.Printf("Warning: Failed to clean log channel: %v", err)
	}

	// Initialize dashboard manager
	dashboardManager := dashboard.NewManager(session, cfg.LogChannelID, cfg.ServerID)

	// Initialize all dashboards
	if err := dashboardManager.Init(); err != nil {
		log.Fatalf("Failed to initialize dashboards: %v", err)
	}

	// Force initial update to populate server info immediately
	if err := dashboardManager.Server.ForceUpdate(); err != nil {
		log.Printf("Warning: Failed to update server dashboard: %v", err)
	}

	fmt.Println("Dashboards initialized!")

	// TODO: Initialize Redis cache
	// TODO: Setup event handlers

	fmt.Println("Dexter Discord Interface running...")

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	fmt.Println("Shutting down...")

	// Cleanup dashboards
	if err := dashboardManager.Shutdown(); err != nil {
		log.Printf("Error during dashboard shutdown: %v", err)
	}

	fmt.Println("Shutdown complete.")
}
