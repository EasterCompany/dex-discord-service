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

func registerEventHandlers(s *discordgo.Session, eventHandler *events.Handler) {
	s.AddHandler(eventHandler.Ready)
	s.AddHandler(eventHandler.MessageCreate)
}

func initCache(cfg *config.CacheConfig, logger logger.Logger) (cache.Cache, cache.Cache) {
	localCache, err := cache.New(cfg.Local)
	if err != nil {
		// Log the error but don't exit; the bot can run without a cache in a degraded state.
		logger.Error("Failed to initialize local cache", err)
	}
	cloudCache, _ := cache.New(cfg.Cloud) // For health check
	return localCache, cloudCache
}

func initDiscord(token string) (*discordgo.Session, error) {
	s, err := session.NewSession(token)
	if err != nil {
		return nil, fmt.Errorf("Error creating Discord session: %w", err)
	}
	return s, nil
}

func loadConfig() (*config.AllConfig, error) {
	cfg, err := config.LoadAllConfigs()
	if err != nil {
		return nil, fmt.Errorf("Fatal error loading config: %w", err)
	}
	return cfg, nil
}

// main orchestrates the bot's startup, operation, and graceful shutdown.
func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf(err.Error())
	}

	s, err := initDiscord(cfg.Discord.Token)
	if err != nil {
		log.Fatalf(err.Error())
	}

	logger := logger.NewLogger(s, cfg.Discord.LogChannelID)

	localCache, cloudCache := initCache(cfg.Cache, logger)

	stateManager := events.NewStateManager()

	eventHandler := events.NewHandler(localCache, cfg.Discord, cfg.Bot, s, logger, stateManager)

	registerEventHandlers(s, eventHandler)

	if err = s.Open(); err != nil {
		logger.Fatal("Error opening connection to Discord", err)
	}

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

	cleanupReport := performCleanup(s, localCache, cfg.Discord, bootMessageID, logger)
	updateBootMessage("`Dexter` is starting up...\n✅ Discord connection established\n✅ Caches initialized\n✅ Cleanup complete")

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
				stateManager.GetOrStoreGuildState(guildID)
			}
		}
	}
	updateBootMessage("`Dexter` is starting up...\n✅ Discord connection established\n✅ Caches initialized\n✅ Cleanup complete\n✅ Guild states loaded")

	performHealthCheck(s, localCache, cloudCache, cfg, bootMessageID, cleanupReport, logger)

	waitForShutdown()

	s.Close()
	fmt.Println("\nBot shutting down.")
}

func waitForShutdown() {
	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
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
func performCleanup(s *discordgo.Session, localCache cache.Cache, discordCfg *config.DiscordConfig, bootMessageID string, logger logger.Logger) string {
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
func performHealthCheck(s *discordgo.Session, localCache, cloudCache cache.Cache, cfg *config.AllConfig, bootMessageID, cleanupReport string, logger logger.Logger) {
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
