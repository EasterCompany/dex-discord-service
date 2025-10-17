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

	// Create a new Discord session using the token from the config
	s, err := session.NewSession(cfg.Discord.Token)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	// Initialize the logger
	logger.Init(s, cfg.Discord.LogChannelID)

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

	if bootMessage != nil {
		logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ Websocket connection opened")
	}

	// Health and system checks
	go func() {
		var cpuUsages []float64
		var memUsages []float64
		startTime := time.Now()
		var deletedMessages int

		for time.Since(startTime) < 10*time.Second {
			// Clear channel messages
			if cfg.Discord.LogChannelID != "" {
				messages, err := s.ChannelMessages(cfg.Discord.LogChannelID, 100, "", "", "")
				if err == nil {
					var messageIDs []string
					for _, msg := range messages {
						// Do not delete the boot message
						if bootMessage != nil && msg.ID == bootMessage.ID {
							continue
						}
						messageIDs = append(messageIDs, msg.ID)
					}
					if len(messageIDs) > 0 {
						s.ChannelMessagesBulkDelete(cfg.Discord.LogChannelID, messageIDs)
						deletedMessages += len(messageIDs)
					}
				}
			}

			cpuUsage, _ := system.GetCPUUsage()
			memUsage, _ := system.GetMemoryUsage()
			cpuUsages = append(cpuUsages, cpuUsage)
			memUsages = append(memUsages, memUsage)

			discordStatus := "**OK**"
			if health.CheckDiscordConnection(s) != nil {
				discordStatus = "**FAILED**"
			}

			status := fmt.Sprintf("**Health & System Status**\nCPU: %.2f%%\nMemory: %.2f%%\nDiscord: %s", cpuUsage, memUsage, discordStatus)
			if bootMessage != nil {
				logger.UpdateInitialMessage(bootMessage.ID, status)
			}

			time.Sleep(1 * time.Second) // Update every second for 10 seconds
		}

		avgCPU := 0.0
		if len(cpuUsages) > 0 {
			for _, u := range cpuUsages {
				avgCPU += u
			}
			avgCPU /= float64(len(cpuUsages))
		}

		avgMem := 0.0
		if len(memUsages) > 0 {
			for _, u := range memUsages {
				avgMem += u
			}
			avgMem /= float64(len(memUsages))
		}

		discordStatus := "**OK**"
		if health.CheckDiscordConnection(s) != nil {
			discordStatus = "**FAILED**"
		}

		finalStatus := fmt.Sprintf("**Health & System Status**\nCPU: %.2f%%\nMemory: %.2f%%\nDiscord: %s", avgCPU, avgMem, discordStatus)
		if deletedMessages > 0 {
			finalStatus += fmt.Sprintf("\nCleared: **%d** messages", deletedMessages)
		}

		if bootMessage != nil {
			logger.UpdateInitialMessage(bootMessage.ID, finalStatus)
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
