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
	buildYear string
	buildHash string
	arch      string
)

func main() {
	// Handle version/help commands first (before flag parsing)
	if len(os.Args) > 1 {
		arg := os.Args[1]
		switch arg {
		case "version", "--version", "-v":
			// Format version like other services: major.minor.patch.branch.commit.buildDate.arch.buildHash
			utils.SetVersion(version, branch, commit, buildDate, buildYear, buildHash, arch)
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
	utils.SetVersion(version, branch, commit, buildDate, buildYear, buildHash, arch)

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

	// Extract Discord token from credentials
	var discordToken string
	if creds, ok := selfConfig.Credentials.(map[string]interface{}); ok {
		if token, found := creds["token"].(string); found {
			discordToken = token
		}
	}
	if discordToken == "" {
		log.Fatalf("FATAL: Discord token not found or invalid in service-map.json for service '%s'", ServiceName)
	}

	// Find the event service configuration from the service map
	var eventServiceConfig *config.ServiceEntry
	if thServices, ok := serviceMap.Services["th"]; ok {
		for _, service := range thServices {
			if service.ID == "dex-event-service" {
				eventServiceConfig = &service
				break
			}
		}
	}
	if eventServiceConfig == nil {
		log.Fatalf("FATAL: Event service 'dex-event-service' not found in service-map.json. Shutting down.")
	}
	eventServiceURL := fmt.Sprintf("http://%s:%s", eventServiceConfig.Domain, eventServiceConfig.Port)

	// Create a context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the core event logic in a goroutine
	go func() {
		log.Println("Core Logic: Starting...")
		if err := RunCoreLogic(ctx, discordToken, eventServiceURL); err != nil {
			log.Printf("Core Logic Error: %v", err)
			// Trigger shutdown if core logic fails
			cancel()
		}
		log.Println("Core Logic: Stopped")
	}()

	// Configure HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      nil, // Uses DefaultServeMux
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Register handlers
	// /service endpoint is public (for health checks)
	http.HandleFunc("/service", endpoints.ServiceHandler)

	// /post endpoint is protected by auth middleware
	http.HandleFunc("/post", middleware.ServiceAuthMiddleware(endpoints.PostHandler))

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
