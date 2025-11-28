package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
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
	// Globals for Discord bot functionality
	discordSession *discordgo.Session
	voiceManager   *handlers.VoiceConnectionManager // Now global to be accessible for shutdown

	// Version information, injected at build time
	version   string
	branch    string
	commit    string
	buildDate string
	buildHash string
	arch      string
)

func main() {
	// Format the full version string using the same logic as other services
	formattedArch := strings.ReplaceAll(arch, "/", "-")
	vParts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	major, minor, patch := "0", "0", "0"
	if len(vParts) >= 3 {
		major = vParts[0]
		minor = vParts[1]
		patch = vParts[2]
	}
	fullVersion := fmt.Sprintf("%s.%s.%s.%s.%s.%s.%s.%s",
		major, minor, patch, branch, commit, buildDate, formattedArch, buildHash)

	// Handle CLI arguments (version, help, etc.) - exit after displaying
	if len(os.Args) > 1 {
		arg := os.Args[1]
		switch arg {
		case "--version", "-v", "version":
			fmt.Println(fullVersion)
			os.Exit(0)
		case "--help", "-h", "help":
			fmt.Println("Dexter Discord Service")
			fmt.Println()
			fmt.Println("Usage:")
			fmt.Println("  dex-discord-service          Start the Discord bot and status server")
			fmt.Println("  dex-discord-service version  Display version information")
			fmt.Println("  dex-discord-service help     Display this help message")
			os.Exit(0)
		default:
			fmt.Printf("Unknown argument: %s\n", arg)
			fmt.Println("Use 'dex-discord-service help' for usage information")
			os.Exit(1)
		}
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: Failed to load config, Discord functionality will be disabled: %v", err)
		// If config fails to load, we can still start the status server
		// but with a nil config, so it knows something is wrong.
		cfg = &config.Config{}
	}

	// Initialize status server - THIS MUST ALWAYS RUN
	statusServer := services.NewStatusServer(cfg.ServicePort, fullVersion)
	if err := statusServer.Start(); err != nil {
		log.Fatalf("FATAL: Failed to start status server: %v", err)
	}

	// Start the main Discord bot logic in a goroutine.
	// This goroutine will exit gracefully if the token is missing.
	go runDiscordBot(cfg)

	log.Println("Dexter Discord Service running...")
	log.Println("HTTP status server is available.")

	// Wait for interrupt signal to shut down
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

	// Disconnect from Discord if connected
	if discordSession != nil {
		if err := discordSession.Close(); err != nil {
			log.Printf("Error closing Discord session: %v", err)
		}
	}

	log.Println("Shutdown complete.")
}

func runDiscordBot(cfg *config.Config) {
	startTime := time.Now() // Moved declaration here

	// If token is missing, log a warning and exit this goroutine.
	// The main HTTP server will continue to run.
	if cfg.DiscordToken == "" {
		log.Println("Warning: Discord token not configured. Discord bot functionality is disabled.")
		return
	}

	log.Println("Discord token found, starting bot...")

	// Initialize Redis client
	redisClient, err := cache.NewRedisClient(cfg)
	if err != nil {
		log.Printf("Error: Failed to initialize Redis client, bot cannot start: %v", err)
		return
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Printf("Error closing Redis client: %v", err)
		}
	}()
	log.Println("Connected to Redis for Discord bot.")

	// Clear Redis cache on boot
	if err := redisClient.ClearCache(context.Background()); err != nil {
		log.Printf("Error: Failed to clear Redis cache: %v", err)
	} else {
		log.Println("Redis cache cleared for Discord bot.")
	}

	log.Println("Creating Discord session...")
	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		log.Printf("Error: Failed to create Discord session: %v", err)
		return
	}
	discordSession = session // Store session in global for shutdown

	session.Identify.Intents = discordgo.IntentsAll
	session.AddHandler(handlers.ConnectHandler())
	session.AddHandler(handlers.DisconnectHandler())

	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Println("Discord session is ready.")

		statusManager := handlers.NewStatusManager(s)
		statusManager.SetSleeping()

		if err := dashboard.CleanLogChannel(s, cfg.LogChannelID); err != nil {
			log.Printf("Warning: Failed to clean log channel: %v", err)
		}

		dashboardManager := dashboard.NewManager(s, cfg.LogChannelID, cfg.ServerID, redisClient, cfg)
		if err := dashboardManager.Init(); err != nil {
			log.Printf("Error: Failed to initialize dashboards: %v", err)
			return
		}

		if err := dashboardManager.Server.ForceUpdate(); err != nil {
			log.Printf("Warning: Failed to update server dashboard: %v", err)
		}
		log.Println("Dashboards initialized!")

		if _, err := s.ChannelMessageSend(cfg.LogChannelID, "🤖 **Dexter Discord Service** is now online!"); err != nil {
			log.Printf("Warning: Failed to send startup notification: %v", err)
		} else {
			log.Println("Startup notification sent to admin channel!")
		}

		voiceManager = handlers.NewVoiceConnectionManager(s, dashboardManager.Voice, dashboardManager.Events, dashboardManager.Logs)
		log.Println("Voice connection manager initialized!")

		healthMonitor := handlers.NewHealthMonitor(cfg.ServerID, statusManager, dashboardManager.Logs, dashboardManager.Events)
		healthMonitor.SetRedisReady(true)
		healthMonitor.SetDashboardsReady(true)
		healthMonitor.SetDiscordReady(true)
		healthMonitor.Start()
		log.Println("Health monitor started!")

		snapshotBuilder := contextpkg.NewSnapshotBuilder(s, dashboardManager, redisClient, cfg, startTime)
		log.Println("Context snapshot builder initialized!")

		commandHandler := commands.NewHandler(s, cfg, dashboardManager, redisClient)
		log.Println("Command handler initialized!")

		s.AddHandler(handlers.MessageCreateHandler(dashboardManager.Messages, statusManager, snapshotBuilder))
		s.AddHandler(handlers.GenericEventHandler(dashboardManager.Events, dashboardManager.Voice, dashboardManager.VoiceState))
		s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) { commandHandler.HandleCommand(m) })
		log.Println("All event handlers registered.")
	})

	log.Println("Opening Discord connection...")
	if err := session.Open(); err != nil {
		log.Printf("Error: Failed to open Discord connection: %v", err)
		return
	}
	log.Println("Discord connection opened successfully.")
}
