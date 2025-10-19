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
	"github.com/EasterCompany/dex-discord-interface/stt"
	"github.com/EasterCompany/dex-discord-interface/system"
	"github.com/bwmarrin/discordgo"
)

func registerEventHandlers(s *discordgo.Session, eventHandler *events.Handler) {
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		addedMessagesCount, addedMessagesSize := eventHandler.Ready(s, r)
		// Store these values for the health check report
		eventHandler.StateManager.SetAddedMessagesStats(addedMessagesCount, addedMessagesSize)
	})
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
		return nil, fmt.Errorf("error creating Discord session: %w", err)
	}
	return s, nil
}

func initStt(logger logger.Logger) (*stt.Client, error) {
	sttClient, err := stt.NewClient()
	if err != nil {
		return nil, fmt.Errorf("error creating STT client: %w", err)
	}
	return sttClient, nil
}

func loadConfig() (*config.AllConfig, error) {
	cfg, _, err := config.LoadAllConfigs()
	if err != nil {
		return nil, fmt.Errorf("fatal error loading config: %w", err)
	}
	return cfg, nil
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	s, err := initDiscord(cfg.Discord.Token)
	if err != nil {
		log.Fatal(err)
	}

	logger := logger.NewLogger(s, cfg.Discord.LogChannelID)

	localCache, cloudCache := initCache(cfg.Cache, logger)

	stateManager := events.NewStateManager()

	sttClient, err := initStt(logger)
	if err != nil {
		logger.Error("Failed to initialize STT client", err)
	}

	eventHandler := events.NewHandler(localCache, cfg.Discord, cfg.Bot, s, logger, stateManager, sttClient)

	registerEventHandlers(s, eventHandler)

	if err = s.Open(); err != nil {
		logger.Fatal("Error opening connection to Discord", err)
	}

	bootMessage, err := logger.PostInitialMessage("Dexter is starting up...")
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
	updateBootMessage(`Dexter is starting up...
✅ Discord connection established
✅ Caches initialized
✅ STT client initialized`)

	cleanupReport, audioCleanResult, messageCleanResult := performCleanup(s, localCache, cfg.Discord, bootMessageID, logger)
	updateBootMessage(`Dexter is starting up...
✅ Discord connection established
✅ Caches initialized
✅ STT client initialized
✅ Cleanup complete`)

	if localCache != nil {
		guildIDs, err := localCache.GetAllGuildIDs()
		if err != nil {
			logger.Error("Error getting all guild IDs", err)
		} else {
			for _, guildID := range guildIDs {
				stateManager.GetOrStoreGuildState(guildID)
			}
		}
	}
	updateBootMessage(`Dexter is starting up...
✅ Discord connection established
✅ Caches initialized
✅ STT client initialized
✅ Cleanup complete
✅ Guild states loaded`)
	performHealthCheck(s, localCache, cloudCache, cfg, bootMessageID, cleanupReport, audioCleanResult, messageCleanResult, sttClient, stateManager, logger)

	waitForShutdown()

	_ = s.Close()
	fmt.Println("Bot shutting down.")
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
func performCleanup(s *discordgo.Session, localCache cache.Cache, discordCfg *config.DiscordConfig, bootMessageID string, logger logger.Logger) (string, cache.CleanResult, cache.CleanResult) {
	var wg sync.WaitGroup
	results := make(chan cleanup.Result, 3)

	var audioCleanResult cache.CleanResult
	var messageCleanResult cache.CleanResult

	if localCache != nil {
		audioCleanResult, _ = localCache.CleanAllAudio()
		messageCleanResult, _ = localCache.CleanAllMessages()
	}

	wg.Add(3)
	go func() {
		defer wg.Done()
		results <- cleanup.ClearChannel(s, discordCfg.LogChannelID, bootMessageID, discordCfg)
	}()
	go func() {
		defer wg.Done()
		results <- cleanup.ClearChannel(s, discordCfg.TranscriptionChannelID, "", discordCfg)
	}()
	go func() { defer wg.Done(); results <- cleanup.CleanStaleMessages(s, discordCfg.TranscriptionChannelID) }()
	wg.Wait()
	close(results)

	cleanupStats := make(map[string]int)
	for result := range results {
		cleanupStats[result.Name] += result.Count
	}

	reportFields := []string{
		"**House Keeping**",
		fmt.Sprintf("🧹 Logs: `%d` removed.", cleanupStats["ClearLogs"]),
		fmt.Sprintf("🧹 Transcriptions: `%d` removed.", cleanupStats["ClearTranscriptions"]+cleanupStats["CleanStaleMessages"]),
	}
	return strings.Join(reportFields, "\n"), audioCleanResult, messageCleanResult
}

// performHealthCheck runs final system checks and posts the final status message.
func performHealthCheck(s *discordgo.Session, localCache, cloudCache cache.Cache, cfg *config.AllConfig, bootMessageID, cleanupReport string, audioCleanResult, messageCleanResult cache.CleanResult, sttClient *stt.Client, stateManager *events.StateManager, logger logger.Logger) {
	cpuUsage, _ := system.GetCPUUsage()
	memUsage, _ := system.GetMemoryUsage()
	discordStatus := health.GetDiscordStatus(s)
	localCacheStatus := health.GetCacheStatus(localCache, cfg.Cache.Local)
	cloudCacheStatus := health.GetCacheStatus(cloudCache, cfg.Cache.Cloud)
	sttStatus := health.GetSTTStatus(sttClient)

	gpuStatus, gpuInfo := health.GetGPUStatus()

	var gpuInfoStr string
	if gpuInfo != nil {
		gpuInfoStr = fmt.Sprintf(`<:gpu:1429531622478184478> GPU Util: `+"`%.2f%%`"+`
<:gpu:1429531622478184478> GPU Mem: `+"`%.2f%% (%.1fGB / %.1fGB)`",
			gpuInfo.Utilization,
			(gpuInfo.MemoryUsed/gpuInfo.MemoryTotal)*100,
			gpuInfo.MemoryUsed/1024,
			gpuInfo.MemoryTotal/1024,
		)
	} else {
		if gpuStatus != "" {
			gpuInfoStr = fmt.Sprintf("❌ GPU: %s", gpuStatus)
		} else {
			gpuInfoStr = `<:gpu:1429531622478184478> GPU Util: ` + "`-/-`" + `
<:gpu:1429531622478184478> GPU Mem: ` + "`-/-`"
		}
	}

	addedMessagesCount, addedMessagesSize := stateManager.GetAddedMessagesStats()

	statusFields := []string{
		"**System Status**",
		fmt.Sprintf("<:cpu:1429533473365823628> CPU: `%.2f%%`", cpuUsage),
		fmt.Sprintf("<:ram:1429533495633510461> Memory: `%.2f%%`", memUsage),
		gpuInfoStr,
		"",
		"**Service Status**",
		fmt.Sprintf("<:discord:1429533475303719013> Discord: %s", discordStatus),
		fmt.Sprintf("<:redis:1429533496954585108> Local Cache: %s", localCacheStatus),
		fmt.Sprintf("<:quickredis:1429533493934948362> Cloud Cache: %s", cloudCacheStatus),
		fmt.Sprintf("🗣️ STT Client: %s", sttStatus),
		"",
		cleanupReport,
		fmt.Sprintf("🧹 Audio Cache: `+%d (%s)` / `-%d (%s)`", 0, humanReadableBytes(0), audioCleanResult.Count, humanReadableBytes(audioCleanResult.BytesFreed)), // Added audio count is not tracked yet
		fmt.Sprintf("🧹 Message Cache: `+%d (%s)` / `-%d (%s)`", addedMessagesCount, humanReadableBytes(addedMessagesSize), messageCleanResult.Count, humanReadableBytes(messageCleanResult.BytesFreed)),
	}

	finalStatus := strings.Join(statusFields, "\n")
	if bootMessageID != "" {
		logger.UpdateInitialMessage(bootMessageID, finalStatus)
	}
}
