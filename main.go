package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/cleanup"
	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/events"
	"github.com/EasterCompany/dex-discord-interface/health"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/EasterCompany/dex-discord-interface/session"
	"github.com/EasterCompany/dex-discord-interface/system"
	"github.com/bwmarrin/discordgo"
)

// main orchestrates the bot's startup, operation, and graceful shutdown.
func main() {
	// 1. Load Configuration
	cfg, err := config.LoadAllConfigs()
	if err != nil {
		log.Fatalf("Fatal error loading config: %v", err)
	}

	// 2. Initialize Discord Session
	s, err := session.NewSession(cfg.Discord.Token)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	// 3. Initialize Logger
	logger.Init(s, cfg.Discord.LogChannelID)

	// 4. Initialize Cache Service (before handlers)
	localCache, err := cache.New(cfg.Cache.Local)
	if err != nil {
		// Log the error but don't exit; the bot can run without a cache in a degraded state.
		logger.Error("Failed to initialize local cache", err)
	}
	cloudCache, _ := cache.New(cfg.Cache.Cloud) // For health check

	// 5. Create Event Handler with all dependencies
	eventHandler := events.NewHandler(localCache, cfg.Discord)

	// 6. Register Event Handlers
	s.AddHandler(eventHandler.Ready)
	s.AddHandler(eventHandler.MessageCreate)
	s.AddHandler(eventHandler.SpeakingUpdate)

	// 7. Connect to Discord
	if err = s.Open(); err != nil {
		logger.Fatal("Error opening connection to Discord", err)
	}

	// 8. Post Initial Boot Message (this will now work correctly)
	bootMessage, err := logger.PostInitialMessage("`Dexter` is starting up...")
	if err != nil {
		logger.Error("Failed to post initial boot message", err)
	}
	bootMessageID := ""
	if bootMessage != nil {
		bootMessageID = bootMessage.ID
	}
	updateBootMessage := func(content string) {
		if bootMessage != nil {
			logger.UpdateInitialMessage(bootMessageID, content)
		}
	}
	updateBootMessage("`Dexter` is starting up...\n✅ Discord connection established\n✅ Caches initialized")

	// 9. Perform Boot-time Cleanup
	cleanupReport := performCleanup(s, localCache, cfg.Discord, bootMessageID)
	updateBootMessage("`Dexter` is starting up...\n✅ Discord connection established\n✅ Caches initialized\n✅ Cleanup complete")

	// 10. Load Persistent State
	if localCache != nil {
		guildIDs, err := localCache.GetAllGuildIDs()
		if err != nil {
			logger.Error("Error getting all guild IDs", err)
		} else {
			for _, guildID := range guildIDs {
				state, err := localCache.LoadGuildState(guildID)
				if err != nil {
					logger.Error(fmt.Sprintf("Error loading guild state for guild %s", guildID), err)
					continue
				}
				events.LoadGuildState(guildID, state)
			}
		}
	}
	updateBootMessage("`Dexter` is starting up...\n✅ Discord connection established\n✅ Caches initialized\n✅ Cleanup complete\n✅ Guild states loaded")

	// 11. Final Health Check and Ready Message
	performHealthCheck(s, localCache, cloudCache, cfg, bootMessageID, cleanupReport)

	// 12. Wait for shutdown signal
	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly close down
	s.Close()
	fmt.Println("\nBot shutting down.")
}

// humanReadableBytes converts a size in bytes to a human-readable string.
func humanReadableBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// performCleanup runs all boot-time cleanup tasks and returns a formatted report.
func performCleanup(s *discordgo.Session, localCache cache.Cache, discordCfg *config.DiscordConfig, bootMessageID string) string {
	var wg sync.WaitGroup
	results := make(chan cleanup.Result, 3)

	var audioCleanResult cache.CleanResult
	var messageCleanResult cache.CleanResult

	if localCache != nil {
		audioCleanResult, _ = localCache.CleanAllAudio()
		messageCleanResult, _ = localCache.CleanAllMessages()
	}

	wg.Add(3)
	go func() { defer wg.Done(); results <- cleanup.ClearChannel(s, discordCfg.LogChannelID, bootMessageID) }()
	go func() { defer wg.Done(); results <- cleanup.ClearChannel(s, discordCfg.TranscriptionChannelID, "") }()
	go func() { defer wg.Done(); results <- cleanup.CleanStaleMessages(s, discordCfg.TranscriptionChannelID) }()
	wg.Wait()
	close(results)

	cleanupStats := make(map[string]int)
	for result := range results {
		cleanupStats[result.Name] += result.Count
	}

	reportFields := []string{
		"**House Keeping**",
		fmt.Sprintf("🧹 Logs Channel: `%d` logs removed.", cleanupStats["ClearLogs"]),
		fmt.Sprintf("🧹 Transcriptions Channel: `%d` transcriptions removed.", cleanupStats["ClearTranscriptions"]+cleanupStats["CleanStaleMessages"]),
		fmt.Sprintf("🧹 Audio Cache: `%s` (%d values) freed.", humanReadableBytes(audioCleanResult.BytesFreed), audioCleanResult.Count),
		fmt.Sprintf("🧹 Message Cache: `%s` (%d values) freed.", humanReadableBytes(messageCleanResult.BytesFreed), messageCleanResult.Count),
	}
	return strings.Join(reportFields, "\n")
}

// performHealthCheck runs final system checks and posts the final status message.
func performHealthCheck(s *discordgo.Session, localCache, cloudCache cache.Cache, cfg *config.AllConfig, bootMessageID, cleanupReport string) {
	cpuUsage, _ := system.GetCPUUsage()
	memUsage, _ := system.GetMemoryUsage()
	discordStatus := health.GetDiscordStatus(s)
	localCacheStatus := health.GetCacheStatus(localCache, cfg.Cache.Local)
	cloudCacheStatus := health.GetCacheStatus(cloudCache, cfg.Cache.Cloud)

	statusFields := []string{
		"**System Status**",
		fmt.Sprintf("💻 CPU: `%.2f%%`", cpuUsage),
		fmt.Sprintf("🧠 Memory: `%.2f%%`", memUsage),
		"",
		"**Service Status**",
		fmt.Sprintf("🤖 Discord: %s", discordStatus),
		fmt.Sprintf("🏠 Local Cache: %s", localCacheStatus),
		fmt.Sprintf("☁️ Cloud Cache: %s", cloudCacheStatus),
		"",
		cleanupReport,
	}

	finalStatus := strings.Join(statusFields, "\n")
	if bootMessageID != "" {
		logger.UpdateInitialMessage(bootMessageID, finalStatus)
	}
}
