package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
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
	"github.com/EasterCompany/dex-discord-interface/worker"
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
		logger.Error("opening discord connection", err)
		os.Exit(1)
	}

	// Initial boot message
	bootMessage, err := logger.PostInitialMessage("Initiating bot instance...")
	if err != nil {
		// Don't use the custom logger here, as it might be the source of the error.
		log.Printf("Error posting initial message: %v", err)
		bootMessage = nil
	}

	if bootMessage != nil {
		logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ Config loaded")
	}

	// Initialize the database
	db, err := database.New(cfg.Redis.Addr)
	if err != nil {
		logger.Error("initializing database", err)
		os.Exit(1)
	}
	if bootMessage != nil {
		logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ Database initialized")
	}

	// Clean up old file-based storage
	if err := store.Cleanup(); err != nil {
		logger.Error("cleaning up old storage", err)
	}
	if bootMessage != nil {
		logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ Old storage cleaned up")
	}

	// Load all guild states from the database
	guildIDs, err := db.GetAllGuildIDs()
	if err != nil {
		logger.Error("getting all guild IDs", err)
	} else {
		if bootMessage != nil {
			logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ Guild states loaded")
		}

		for _, guildIDKey := range guildIDs {
			guildID := strings.Split(strings.Split(guildIDKey, ":")[1], ":")[0]
			state, err := db.LoadGuildState(guildID)
			if err != nil {
				logger.Error(fmt.Sprintf("loading guild state for guild %s", guildID), err)
				continue
			}
			events.LoadGuildState(guildID, state)
		}
	}

	// Initialize the Google Speech-to-Text service
	sttClient, err := stt.New()
	if err != nil {
		logger.Error("initializing STT service", err)
		os.Exit(1)
	}
	defer sttClient.Close()
	if bootMessage != nil {
		logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ STT service initialized")
	}

	// Initialize and start the worker pool
	// We'll use number of CPU cores as the number of workers and a queue size of 100.
	workerPool := worker.New(runtime.NumCPU(), 100)
	workerPool.Start()

	if bootMessage != nil {
		logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ Transcription worker pool started")
	}

	if bootMessage != nil {
		logger.UpdateInitialMessage(bootMessage.ID, bootMessage.Content+"\n✅ Discord session created")
	}

	// Initialize the events module with the database and stt clients
	events.Init(db, sttClient, cfg.Discord, workerPool)

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
				if err != nil {
					logger.Error("clearing log channel messages", err)
				} else {
					var messageIDs []string
					for _, msg := range messages {
						// Do not delete the boot message
						if bootMessage != nil && msg.ID == bootMessage.ID {
							continue
						}
						messageIDs = append(messageIDs, msg.ID)
					}
					if len(messageIDs) > 0 {
						if err := s.ChannelMessagesBulkDelete(cfg.Discord.LogChannelID, messageIDs); err != nil {
							logger.Error("bulk deleting log channel messages", err)
						} else {
							deletedMessages += len(messageIDs)
						}
					}
				}
			}

			cpuUsage, err := system.GetCPUUsage()
			if err != nil {
				logger.Error("getting CPU usage", err)
			}
			memUsage, err := system.GetMemoryUsage()
			if err != nil {
				logger.Error("getting Memory usage", err)
			}
			cpuUsages = append(cpuUsages, cpuUsage)
			memUsages = append(memUsages, memUsage)

			discordStatus := "**OK**"
			if err := health.CheckDiscordConnection(s); err != nil {
				discordStatus = "**FAILED**"
				logger.Error("checking discord connection", err)
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
		if err := health.CheckDiscordConnection(s); err != nil {
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
