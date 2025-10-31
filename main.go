package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/EasterCompany/dex-discord-interface/config"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("Loaded config for server: %s\n", cfg.ServerID)
	fmt.Printf("Log channel: %s\n", cfg.LogChannelID)
	fmt.Printf("Redis: %s (db: %d)\n", cfg.RedisAddr, cfg.RedisDB)

	// TODO: Initialize Discord client
	// TODO: Initialize Redis cache
	// TODO: Initialize dashboards

	fmt.Println("Dexter Discord Interface starting...")

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	fmt.Println("Shutting down...")
}
