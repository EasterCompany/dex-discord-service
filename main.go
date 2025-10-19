package main

import (
	"bytes"
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
	"github.com/EasterCompany/dex-discord-interface/llm"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/EasterCompany/dex-discord-interface/session"
	"github.com/EasterCompany/dex-discord-interface/stt"
	"github.com/EasterCompany/dex-discord-interface/system"
	"github.com/bwmarrin/discordgo"
)

func main() {
	cfg, err := config.LoadAllConfigs()
	if err != nil {
		log.Fatalf("Fatal error loading config: %v", err)
	}

	s, err := session.NewSession(cfg.Discord.Token)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	logger := logger.NewLogger(s, cfg.Discord.LogChannelID)
	localCache, cloudCache := initCache(cfg.Cache, logger)
	stateManager := events.NewStateManager()

	sttClient, err := stt.NewClient()
	if err != nil {
		logger.Error("Failed to initialize STT client", err)
	}

	llmClient, err := llm.NewClient(cfg.Persona, localCache)
	if err != nil {
		logger.Fatal("Failed to initialize LLM client", err)
	}

	eventHandler := events.NewHandler(localCache, cfg.Discord, cfg.Bot, s, logger, stateManager, sttClient, llmClient)

	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		count, size := eventHandler.Ready(s, r)
		stateManager.SetAddedMessagesStats(count, size)
	})
	s.AddHandler(eventHandler.MessageCreate)

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
✅ STT client initialized
✅ LLM client initialized`)

	cleanupReport, audioCleanResult, messageCleanResult := performCleanup(s, localCache, cfg.Discord, bootMessageID, logger)
	updateBootMessage(`Dexter is starting up...
✅ Discord connection established
✅ Caches initialized
✅ STT client initialized
✅ LLM client initialized
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
✅ LLM client initialized
✅ Cleanup complete
✅ Guild states loaded`)

	postFinalStatus(s, localCache, cloudCache, cfg, bootMessageID, cleanupReport, audioCleanResult, messageCleanResult, sttClient, stateManager, logger, llmClient.SystemPrompt)

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	_ = s.Close()
	fmt.Println("Bot shutting down.")
}

func initCache(cfg *config.CacheConfig, logger logger.Logger) (cache.Cache, cache.Cache) {
	localCache, err := cache.New(cfg.Local)
	if err != nil {
		logger.Error("Failed to initialize local cache", err)
	}
	cloudCache, _ := cache.New(cfg.Cloud)
	return localCache, cloudCache
}

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

func postFinalStatus(s *discordgo.Session, localCache, cloudCache cache.Cache, cfg *config.AllConfig, bootMessageID, cleanupReport string, audioCleanResult, messageCleanResult cache.CleanResult, sttClient *stt.Client, stateManager *events.StateManager, logger logger.Logger, systemPrompt string) {
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
	activeGuildsStrings := health.GetFormattedActiveGuilds(s)
	finalStatus := strings.Join([]string{
		"**System Status**",
		fmt.Sprintf("🖥️ CPU: `%.2f%%`", cpuUsage),
		fmt.Sprintf("<:ram:1429533495633510461> Memory: `%.2f%%`", memUsage),
		gpuInfoStr,
		"",
		"**Service Status**",
		fmt.Sprintf("<:discord:1429533475303719013> Discord: %s", discordStatus),
		fmt.Sprintf("🎧 STT Client: %s", sttStatus),
		fmt.Sprintf("<:redis:1429533496954585108> Local Cache: %s", localCacheStatus),
		fmt.Sprintf("<:quickredis:1429533493934948362> Cloud Cache: %s", cloudCacheStatus),
		"",
		cleanupReport,
		"",
		"**Essential Tasks**",
		fmt.Sprintf("🗘 Audio Cache: `+%d (%s)` / `-%d (%s)`", 0, humanReadableBytes(0), audioCleanResult.Count, humanReadableBytes(audioCleanResult.BytesFreed)),
		fmt.Sprintf("🗘 Message Cache: `+%d (%s)` / `-%d (%s)`", addedMessagesCount, humanReadableBytes(addedMessagesSize), messageCleanResult.Count, humanReadableBytes(messageCleanResult.BytesFreed)),
		"",
		strings.Join(activeGuildsStrings, "\n"),
	}, "\n")

	if bootMessageID != "" {
		_ = s.ChannelMessageDelete(cfg.Discord.LogChannelID, bootMessageID)
	}

	_, _ = s.ChannelFileSendWithMessage(
		cfg.Discord.LogChannelID,
		finalStatus,
		"persona.md",
		bytes.NewBufferString(systemPrompt),
	)
}
