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

	// 4. Connect to Discord
	if err = s.Open(); err != nil {
		logger.Fatal("Error opening connection to Discord", err)
	}

	// 5. Post Initial Boot Message
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

	updateBootMessage("`Dexter` is starting up...\n✅ Discord connection established")

	// 6. Initialize Cache Services
	localCache, err := cache.New(cfg.Cache.Local)
	if err != nil {
		logger.Error("Failed to initialize local cache", err)
	}
	cloudCache, _ := cache.New(cfg.Cache.Cloud) // Initialized for health check, but not used in core logic
	updateBootMessage("`Dexter` is starting up...\n✅ Discord connection established\n✅ Caches initialized")

	// 7. Perform Boot-time Cleanup
	performCleanup(s, localCache, cfg.Discord, bootMessageID)
	updateBootMessage("`Dexter` is starting up...\n✅ Discord connection established\n✅ Caches initialized\n✅ Cleanup complete")

	// 8. Load Persistent State (e.g., Guild States)
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

	// 9. Initialize Event Handlers
	events.Init(localCache, cfg.Discord)
	s.AddHandler(events.Ready)
	s.AddHandler(events.MessageCreate)
	s.AddHandler(events.SpeakingUpdate)

	// 10. Final Health Check and Ready Message
	performHealthCheck(s, localCache, cloudCache, cfg, bootMessageID)

	// 11. Wait for shutdown signal
	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly close down
	s.Close()
	fmt.Println("\nBot shutting down.")
}

// performCleanup runs all boot-time cleanup tasks.
func performCleanup(s *discordgo.Session, localCache cache.Cache, discordCfg *config.DiscordConfig, bootMessageID string) {
	var wg sync.WaitGroup
	cleanupResults := make(chan cleanup.Result, 3)
	var cleanedAudioCount int64

	if localCache != nil {
		var audioErr error
		cleanedAudioCount, audioErr = localCache.CleanAllAudio()
		if audioErr != nil {
			logger.Error("Error cleaning up orphaned audio data from cache", audioErr)
		}
	}

	wg.Add(3)
	go func() {
		defer wg.Done()
		cleanupResults <- cleanup.ClearChannel(s, discordCfg.LogChannelID, bootMessageID)
	}()
	go func() {
		defer wg.Done()
		cleanupResults <- cleanup.ClearChannel(s, discordCfg.TranscriptionChannelID, "")
	}()
	go func() {
		defer wg.Done()
		cleanupResults <- cleanup.CleanStaleMessages(s, discordCfg.TranscriptionChannelID)
	}()
	wg.Wait()
	close(cleanupResults)

	// Process results for logging, though not displayed in final message anymore for simplicity.
	cleanupStats := make(map[string]int)
	for result := range cleanupResults {
		cleanupStats[result.Name] += result.Count
	}
	log.Printf("Cleanup complete: %+v, audio files: %d", cleanupStats, cleanedAudioCount)
}

// performHealthCheck runs final system checks and posts the final status message.
func performHealthCheck(s *discordgo.Session, localCache, cloudCache cache.Cache, cfg *config.AllConfig, bootMessageID string) {
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
	}

	finalStatus := strings.Join(statusFields, "\n")
	if bootMessageID != "" {
		logger.UpdateInitialMessage(bootMessageID, finalStatus)
	}
}
