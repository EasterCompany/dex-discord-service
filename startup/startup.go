package startup

import (
	"fmt"
	"strings"
	"sync"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/cleanup"
	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/events"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/bwmarrin/discordgo"
)

func PerformCleanup(s *discordgo.Session, localCache cache.Cache, discordCfg *config.DiscordConfig, bootMessageID string, logger logger.Logger) (string, cache.CleanResult, cache.CleanResult) {
	var wg sync.WaitGroup
	results := make(chan cleanup.Result, 3)

	var audioCleanResult cache.CleanResult
	var messageCleanResult cache.CleanResult

	if localCache != nil {
		audioCleanResult, _ = localCache.CleanAllAudio()
	}

	go func() {
		defer wg.Done()
		results <- cleanup.ClearChannel(s, discordCfg.LogChannelID, bootMessageID, discordCfg)
	}()
	wg.Wait()
	close(results)

	cleanupStats := make(map[string]int)
	for result := range results {
		cleanupStats[result.Name] += result.Count
	}

	reportFields := []string{
		"**House Keeping**",
		fmt.Sprintf("🧹 Logs: `%d` removed.", cleanupStats["ClearLogs"]),
	}
	return strings.Join(reportFields, "\n"), audioCleanResult, messageCleanResult
}

func LoadGuildStates(localCache cache.Cache, stateManager *events.StateManager, logger logger.Logger) {
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
}

func CacheAllDMs(s *discordgo.Session, c cache.Cache, logger logger.Logger) {
	if c == nil {
		return
	}
	dmChannels, err := c.GetAllDMChannels()
	if err != nil {
		logger.Error("Error getting all DM channels", err)
		return
	}

	for _, channelID := range dmChannels {
		messages, err := s.ChannelMessages(channelID, 50, "", "", "")
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to fetch messages for DM channel %s", channelID), err)
			continue
		}
		key := fmt.Sprintf("messages:dm:%s", channelID)
		if err := c.BulkInsertMessages(key, messages); err != nil {
			logger.Error(fmt.Sprintf("Failed to bulk insert messages for DM channel %s", channelID), err)
		}
	}
}
