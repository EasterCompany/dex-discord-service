package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/database"
	"github.com/EasterCompany/dex-discord-interface/events"
	"github.com/EasterCompany/dex-discord-interface/health"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/EasterCompany/dex-discord-interface/session"
	"github.com/EasterCompany/dex-discord-interface/store"
	"github.com/EasterCompany/dex-discord-interface/stt"
	"github.com/EasterCompany/dex-discord-interface/system"
)

func main() {
	// Load configuration
	cfg, err := config.LoadAllConfigs()
	if err != nil {
		log.Fatalf("Fatal error loading config: %v", err)
	}
	log.Printf("Discord token loaded: %s", cfg.Discord.Token)

	// Create a new Discord session using the token from the config
	s, err := session.NewSession(cfg.Discord.Token)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	// Open a websocket connection to Discord and begin listening
	err = s.Open()
	if err != nil {
		log.Fatalf("Error opening connection: %v", err)
	}

	// Initialize the logger
	logger.Init(s, cfg.Discord.LogChannelID)

	// Initial boot message
	bootMessage, err := logger.PostInitialMessage("Initiating bot instance...")
	if err != nil {
		log.Printf("Error posting initial message: %v", err)
		// If we can't post the initial message, we can't update it either.
		// So, we'll just proceed without updating the boot message.
		bootMessage = nil
	}

	if bootMessage != nil {
		logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ Config loaded")
	}

	// Initialize the database
	db, err := database.New(cfg.Redis.Addr)
	if err != nil {
		log.Fatalf("Fatal error initializing database: %v", err)
	}
	if bootMessage != nil {
		logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ Database initialized")
	}

	// Clean up old file-based storage
	if err := store.Cleanup(); err != nil {
		log.Printf("Error cleaning up old storage: %v", err)
	}
	if bootMessage != nil {
		logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ Old storage cleaned up")
	}

	// Load all guild states from the database
	guildIDs, err := db.GetAllGuildIDs()
	if err != nil {
		log.Printf("Error getting all guild IDs: %v", err)
	}
	if bootMessage != nil {
		logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ Guild states loaded")
	}

	for _, guildIDKey := range guildIDs {
		guildID := strings.Split(strings.Split(guildIDKey, ":")[1], ":")[0]
		state, err := db.LoadGuildState(guildID)
		if err != nil {
			log.Printf("Error loading guild state for guild %s: %v", guildID, err)
			continue
		}
		events.LoadGuildState(guildID, state)
	}

	// Initialize the Google Speech-to-Text service
	sttClient, err := stt.New()
	if err != nil {
		log.Fatalf("Fatal error initializing STT service: %v", err)
	}
	defer sttClient.Close()
	if bootMessage != nil {
		logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ STT service initialized")
	}

	if bootMessage != nil {
		logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ Discord session created")
	}

	// Initialize the events module with the database and stt clients
	events.Init(db, sttClient)

	// Add event handlers
	s.AddHandler(events.MessageCreate)
	s.AddHandler(events.SpeakingUpdate) // Use the correct handler for speaking updates

	if bootMessage != nil {
		logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ Websocket connection opened")
	}

	// Health and system checks
	go func() {
		for {
			cpuUsage, _ := system.GetCPUUsage()
			memUsage, _ := system.GetMemoryUsage()
			discordStatus := "✅"
			if health.CheckDiscordConnection(s) != nil {
				discordStatus = "❌"
			}

			status := fmt.Sprintf("\n\n**Health & System Status**\nCPU: %.2f%%\nMemory: %.2f%%\nDiscord: %s", cpuUsage, memUsage, discordStatus)
			if bootMessage != nil {
				logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+status)
			}

			time.Sleep(5 * time.Second)
		}
	}()

	// Wait here until CTRL-C or other term signal is received
	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly close down the Discord session
	s.Close()
	fmt.Println("\nBot shutting down.")
}
