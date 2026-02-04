package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/EasterCompany/dex-discord-service/audio"
	"github.com/EasterCompany/dex-discord-service/config"
	"github.com/EasterCompany/dex-discord-service/endpoints"
	"github.com/EasterCompany/dex-discord-service/middleware"
	"github.com/EasterCompany/dex-discord-service/utils"
)

const ServiceName = "dex-discord-service"

var (
	version   string
	branch    string
	commit    string
	buildDate string
	arch      string
)

func main() {
	// Handle version/help commands first (before flag parsing)
	if len(os.Args) > 1 {
		arg := os.Args[1]
		switch arg {
		case "version", "--version", "-v":
			// Format version like other services: major.minor.patch.branch.commit.buildDate.arch
			utils.SetVersion(version, branch, commit, buildDate, arch)
			fmt.Println(utils.GetVersion().Str)
			os.Exit(0)
		case "help", "--help", "-h":
			fmt.Println("Dexter Discord Service")
			fmt.Println()
			fmt.Println("Usage:")
			fmt.Println("  dex-discord-service              Start the discord service")
			fmt.Println("  dex-discord-service version      Display version information")
			os.Exit(0)
		}
	}

	// Define CLI flags
	flag.Parse()

	// Set the version for the service.
	utils.SetVersion(version, branch, commit, buildDate, arch)

	// Load the service map and find our own configuration.
	serviceMap, err := config.LoadServiceMap()
	if err != nil {
		log.Fatalf("FATAL: Could not load service-map.json: %v", err)
	}

	var selfConfig *config.ServiceEntry
	for _, service := range serviceMap.Services["th"] {
		if service.ID == ServiceName {
			selfConfig = &service
			break
		}
	}

	if selfConfig == nil {
		log.Fatalf("FATAL: Service '%s' not found in service-map.json under 'th' services. Shutting down.", ServiceName)
	}

	// Get port from config, convert to integer.
	port, err := strconv.Atoi(selfConfig.Port)
	if err != nil {
		log.Fatalf("FATAL: Invalid port '%s' for service '%s' in service-map.json: %v", selfConfig.Port, ServiceName, err)
	}

	// Load options.json to get Discord configuration
	options, err := config.LoadOptions()
	if err != nil {
		log.Fatalf("FATAL: Could not load options.json: %v", err)
	}

	// Extract Discord token from options
	discordToken := options.Discord.Token
	if discordToken == "" {
		log.Fatalf("FATAL: Discord token not found or invalid in options.json")
	}

	// Resolve Service URLs
	eventServiceURL := serviceMap.GetServiceURL("dex-event-service", "cs", "8100")
	ttsServiceURL := serviceMap.GetServiceURL("dex-tts-service", "be", "8200")
	sttServiceURL := serviceMap.GetServiceURL("dex-stt-service", "be", "8202")

	// Create a context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Redis Client
	redisClient, err := audio.GetRedisClient(ctx)
	if err != nil {
		log.Printf("Warning: Failed to connect to Redis: %v. User caching will be disabled.", err)
	} else {
		defer func() {
			if err := redisClient.Close(); err != nil {
				log.Printf("Error closing Redis client: %v", err)
			}
		}()
		endpoints.SetRedisClient(redisClient)
	}

	// Initialize Stream Manager
	endpoints.InitStreamManager()

	// Start the core event logic in a goroutine
	go func() {
		log.Println("Core Logic: Starting...")
		endpoints.SetUserConfig(options.Discord.Roles)
		if err := RunCoreLogic(ctx, discordToken, eventServiceURL, ttsServiceURL, sttServiceURL, options.Discord.DefaultVoiceChannel, options.Discord.ServerID, options.Discord.Roles, options.Discord.BuildChannelID, redisClient, port); err != nil {
			log.Printf("Core Logic Error: %v", err)
			// Trigger shutdown if core logic fails
			cancel()
		}
		log.Println("Core Logic: Stopped")
	}()

	// Configure HTTP server
	mux := http.NewServeMux()

	// Register handlers
	// /service endpoint is public (for health checks)
	mux.HandleFunc("/service", endpoints.ServiceHandler)

	// /contacts endpoint is public
	mux.HandleFunc("/contacts", endpoints.GetContactsHandler)

	// /channels endpoint is public
	mux.HandleFunc("/channels", endpoints.ListChannelsHandler)

	// /profile/ endpoint is public for GET, protected for POST
	mux.HandleFunc("/profile/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			middleware.ServiceAuthMiddleware(endpoints.UpdateProfileHandler)(w, r)
		} else {
			endpoints.GetProfileHandler(w, r)
		}
	})

	// /post endpoint is protected by auth middleware
	mux.HandleFunc("/post", middleware.ServiceAuthMiddleware(endpoints.PostHandler))

	// Streaming endpoints
	mux.HandleFunc("/message/stream/start", middleware.ServiceAuthMiddleware(endpoints.StartStreamHandler))
	mux.HandleFunc("/message/stream/update", middleware.ServiceAuthMiddleware(endpoints.UpdateStreamHandler))
	mux.HandleFunc("/message/stream/complete", middleware.ServiceAuthMiddleware(endpoints.CompleteStreamHandler))

	// /context/channel endpoint is protected by auth middleware
	mux.HandleFunc("/context/channel", middleware.ServiceAuthMiddleware(endpoints.GetChannelContextHandler))

	// /context/guild endpoint is protected by auth middleware
	mux.HandleFunc("/context/guild", middleware.ServiceAuthMiddleware(endpoints.GetGuildStructureHandler))

	// /member/ endpoint is protected by auth middleware
	mux.HandleFunc("/member/", middleware.ServiceAuthMiddleware(endpoints.GetMemberHandler))

	// /channel/latest endpoint is protected by auth middleware
	mux.HandleFunc("/channel/latest", middleware.ServiceAuthMiddleware(endpoints.GetLatestMessageIDHandler))

	// /channel/voice/count endpoint is protected by auth middleware
	mux.HandleFunc("/channel/voice/count", middleware.ServiceAuthMiddleware(endpoints.GetVoiceChannelUserCountHandler))

	// /status endpoint is protected by auth middleware
	mux.HandleFunc("/status", middleware.ServiceAuthMiddleware(endpoints.UpdateStatusHandler))

	// /typing endpoint is protected by auth middleware
	mux.HandleFunc("/typing", middleware.ServiceAuthMiddleware(endpoints.TypingHandler))

	// /message/delete endpoint is protected by auth middleware
	mux.HandleFunc("/message/delete", middleware.ServiceAuthMiddleware(endpoints.DeleteMessageHandler))

	// /message/react endpoint is protected by auth middleware
	mux.HandleFunc("/message/react", middleware.ServiceAuthMiddleware(endpoints.ReactMessageHandler))

	// /voice/state endpoint is protected by auth middleware
	mux.HandleFunc("/voice/state", middleware.ServiceAuthMiddleware(endpoints.VoiceStateHandler))

	// /audio endpoint is public (for fetching recordings)
	mux.HandleFunc("/audio/", endpoints.AudioHandler)

	// /audio/play endpoint is protected by auth middleware (for streaming TTS)
	mux.HandleFunc("/audio/play", middleware.ServiceAuthMiddleware(endpoints.PlayAudioHandler))

	// /audio/play_music endpoint is protected by auth middleware (for playing YouTube links)
	mux.HandleFunc("/audio/play_music", middleware.ServiceAuthMiddleware(endpoints.PlayMusicHandler))

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      middleware.CorsMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server in a goroutine
	go func() {
		fmt.Printf("Starting %s on :%d\n", ServiceName, port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server crashed: %v", err)
		}
	}()

	// Wait for shutdown signal (SIGTERM from systemd or SIGINT from Ctrl+C)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Block here until signal received
	<-stop
	log.Println("Shutting down service...")

	// Graceful cleanup
	utils.SetHealthStatus("SHUTTING_DOWN", "Service is shutting down")
	cancel() // Signals the core logic to stop

	// Give the HTTP server 5 seconds to finish current requests
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	log.Println("Service exited cleanly")
}
