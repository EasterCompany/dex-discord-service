package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/dashboard"
	"github.com/EasterCompany/dex-discord-interface/handlers"
	"github.com/bwmarrin/discordgo"
)

var (
	dashboardManager *dashboard.Manager
	voiceManager     *handlers.VoiceConnectionManager
	healthMonitor    *handlers.HealthMonitor
	statusManager    *handlers.StatusManager
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("Loaded config for server: %s\n", cfg.ServerID)

	// Initialize Redis client
	redisClient, err := cache.NewRedisClient(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize Redis client: %v", err)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Printf("Error closing Redis client: %v", err)
		}
	}()
	log.Println("Connected to Redis!")

	// Clear Redis cache on boot
	if err := redisClient.ClearCache(context.Background()); err != nil {
		log.Fatalf("Failed to clear Redis cache: %v", err)
	}
	log.Println("Redis cache cleared!")

	// Set up custom log writer to send logs to Redis
	// logWriter := cache.NewLogWriter(redisClient)
	// log.SetOutput(logWriter)
	// log.Println("Log output redirected to Redis.")
	log.Println("About to create discord session")

	log.Println("Creating Discord session...")
	// Create Discord session
	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		log.Fatalf("Failed to create Discord session: %v", err)
	}
	log.Println("Discord session created.")

	// Set intents
	session.Identify.Intents = discordgo.IntentsAll

	// Register connection handlers
	session.AddHandler(handlers.ConnectHandler())
	session.AddHandler(handlers.DisconnectHandler())

	// Add ready handler to initialize dashboards after state is populated
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Println("Discord session is ready.")

		// Initialize status manager and set initial status
		statusManager = handlers.NewStatusManager(s)
		statusManager.SetSleeping()

		// Clean log channel before creating new dashboards
		if err := dashboard.CleanLogChannel(s, cfg.LogChannelID); err != nil {
			log.Printf("Warning: Failed to clean log channel: %v", err)
		}

		// Initialize dashboard manager
		dashboardManager = dashboard.NewManager(s, cfg.LogChannelID, cfg.ServerID, redisClient)

		// Initialize all dashboards
		if err := dashboardManager.Init(); err != nil {
			log.Fatalf("Failed to initialize dashboards: %v", err)
		}

		// Force initial update to populate server info immediately
		if err := dashboardManager.Server.ForceUpdate(); err != nil {
			log.Printf("Warning: Failed to update server dashboard: %v", err)
		}

		log.Println("Dashboards initialized!")

		// Initialize voice connection manager
		voiceManager = handlers.NewVoiceConnectionManager(
			s,
			dashboardManager.Voice,
			dashboardManager.Events,
			dashboardManager.Logs,
		)
		log.Println("Voice connection manager initialized!")

		// Initialize health monitor
		healthMonitor = handlers.NewHealthMonitor(
			cfg.ServerID,
			cfg.DefaultChannelID,
			voiceManager,
			statusManager,
			dashboardManager.Logs,
			dashboardManager.Events,
		)

		// Mark systems as ready
		healthMonitor.SetRedisReady(true)
		healthMonitor.SetDashboardsReady(true)
		healthMonitor.SetDiscordReady(true)

		// Start health monitoring
		healthMonitor.Start()
		log.Println("Health monitor started!")

		// Setup event handlers
		s.AddHandler(handlers.MessageCreateHandler(dashboardManager.Messages, statusManager))
		s.AddHandler(handlers.GenericEventHandler(dashboardManager.Events, dashboardManager.Voice, dashboardManager.VoiceState))
		log.Println("All event handlers registered.")
	})

	log.Println("Opening Discord connection...")
	// Open connection
	if err := session.Open(); err != nil {
		log.Fatalf("Failed to open Discord connection: %v", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			log.Printf("Error closing Discord session: %v", err)
		}
	}()
	log.Println("Discord connection opened.")

	log.Println("Connected to Discord! Waiting for ready event...")

	log.Println("Dexter Discord Interface running...")

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down...")

	// Disconnect from voice if connected
	if voiceManager != nil {
		if err := voiceManager.LeaveVoiceChannel(); err != nil {
			log.Printf("Note: %v", err) // Not necessarily an error if not connected
		}
	}

	// Cleanup dashboards
	if dashboardManager != nil {
		if err := dashboardManager.Shutdown(); err != nil {
			log.Printf("Error during dashboard shutdown: %v", err)
		}
	}

	log.Println("Shutdown complete.")
}
