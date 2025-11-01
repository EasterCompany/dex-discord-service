package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/EasterCompany/dex-discord-service/cache"
	"github.com/EasterCompany/dex-discord-service/commands"
	"github.com/EasterCompany/dex-discord-service/config"
	contextpkg "github.com/EasterCompany/dex-discord-service/context"
	"github.com/EasterCompany/dex-discord-service/dashboard"
	"github.com/EasterCompany/dex-discord-service/handlers"
	"github.com/EasterCompany/dex-discord-service/services"
	"github.com/bwmarrin/discordgo"
)

var (
	dashboardManager *dashboard.Manager
	voiceManager     *handlers.VoiceConnectionManager
	healthMonitor    *handlers.HealthMonitor
	statusManager    *handlers.StatusManager
	snapshotBuilder  *contextpkg.SnapshotBuilder
	healthChecker    *services.HealthChecker
	statusServer     *services.StatusServer
	commandHandler   *commands.Handler
	startTime        time.Time
)

func main() {
	startTime = time.Now()

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

	// Initialize service health checker
	healthChecker = services.NewHealthChecker(10 * time.Second) // Check every 10 seconds

	// Register external services from config with /status endpoints
	for serviceName := range cfg.Services {
		statusURL := cfg.GetServiceStatusURL(serviceName)
		healthChecker.RegisterService(serviceName, statusURL)
	}

	// Start health checker
	healthChecker.Start()
	defer healthChecker.Stop()

	// Initialize status server
	statusServer = services.NewStatusServer(cfg.ServicePort, "1.0.0", healthChecker)
	if err := statusServer.Start(); err != nil {
		log.Fatalf("Failed to start status server: %v", err)
	}

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
		dashboardManager = dashboard.NewManager(s, cfg.LogChannelID, cfg.ServerID, redisClient, cfg)

		// Initialize all dashboards
		if err := dashboardManager.Init(); err != nil {
			log.Fatalf("Failed to initialize dashboards: %v", err)
		}

		// Wire health checker to server dashboard
		dashboardManager.Server.SetHealthChecker(healthChecker)

		// Force initial update to populate server info immediately
		if err := dashboardManager.Server.ForceUpdate(); err != nil {
			log.Printf("Warning: Failed to update server dashboard: %v", err)
		}

		log.Println("Dashboards initialized!")

		// Send startup notification to draw attention to admin channel
		if _, notifErr := s.ChannelMessageSend(cfg.LogChannelID, "🤖 **Dexter Discord Service** is now online!"); notifErr != nil {
			log.Printf("Warning: Failed to send startup notification: %v", notifErr)
		} else {
			log.Println("Startup notification sent to admin channel!")
		}

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

		// Initialize context snapshot builder
		snapshotBuilder = contextpkg.NewSnapshotBuilder(
			s,
			dashboardManager,
			redisClient,
			cfg,
			startTime,
		)
		log.Println("Context snapshot builder initialized!")

		// Initialize command handler
		commandHandler = commands.NewHandler(s, cfg, dashboardManager, redisClient)
		log.Println("Command handler initialized!")

		// Setup event handlers
		s.AddHandler(handlers.MessageCreateHandler(dashboardManager.Messages, statusManager, snapshotBuilder))
		s.AddHandler(handlers.GenericEventHandler(dashboardManager.Events, dashboardManager.Voice, dashboardManager.VoiceState))

		// Setup command handler
		s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
			commandHandler.HandleCommand(m)
		})

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
