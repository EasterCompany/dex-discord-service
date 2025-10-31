package reporting

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/events"
	"github.com/EasterCompany/dex-discord-interface/health"
	"github.com/EasterCompany/dex-discord-interface/interfaces"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/EasterCompany/dex-discord-interface/system"
	"github.com/bwmarrin/discordgo"
)

func humanReadableBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatSystemStatus(sysInfo *system.SysInfo, cpuUsage float64, memUsage float64, gpuInfo []system.GPUInfo) string {
	var gpuInfoStr string
	if len(gpuInfo) > 0 {
		var gpuStrs []string
		for _, gpu := range gpuInfo {
			gpuStrs = append(gpuStrs, fmt.Sprintf("<:nvidia:1431880272110161931> %s: `%.2f%%` (`%s / %s`)", gpu.Name, gpu.Utilization, humanReadableBytes(uint64(gpu.MemoryUsed*1024*1024)), humanReadableBytes(uint64(gpu.MemoryTotal*1024*1024))))
		}
		gpuInfoStr = strings.Join(gpuStrs, "\n")
	} else {
		gpuInfoStr = "❌ No Nvidia GPU Detected."
	}

	return strings.Join([]string{
		"**System Status**",
		fmt.Sprintf("<:cpu:1431892037984194660> %s: `%.2f%%` (`%d cores, %d threads, %.2f GHz`)", sysInfo.CPUModel, cpuUsage, sysInfo.CPUCoreCount, sysInfo.CPUThreadCount, sysInfo.CPUSpeed/1000),
		fmt.Sprintf("<:ram:1429533495633510461> Random Access Memory: `%.2f%%` (`%s / %s`)", memUsage, humanReadableBytes(uint64(memUsage/100*float64(sysInfo.TotalMemory))), humanReadableBytes(sysInfo.TotalMemory)),
		gpuInfoStr,
	}, "\n")
}

func formatStorageStatus(storageInfo []system.StorageInfo) string {
	if len(storageInfo) > 0 {
		var diskStrs []string
		for _, device := range storageInfo {
			if device.MountPoint != "" {
				diskStrs = append(diskStrs, fmt.Sprintf("💿 %s (%s): `%.2f%%` (`%s / %s`)", device.Device, device.MountPoint, device.UsedPercent, humanReadableBytes(device.Used), humanReadableBytes(device.Total)))
			} else {
				diskStrs = append(diskStrs, fmt.Sprintf("💿 %s (not mounted): -- (-- / --)", device.Device))
			}
		}
		return strings.Join([]string{
			"**Storage Devices**",
			strings.Join(diskStrs, "\n"),
		}, "\n")
	}
	return "**Storage Devices**\n❌ No Storage Devices Detected."
}

func formatServiceStatus(discordStatus, sttStatus, llmServerStatus, localCacheStatus, cloudCacheStatus string) string {
	return strings.Join([]string{
		"**Service Status**",
		fmt.Sprintf("<:discord:1429533475303719013> Discord: %s", discordStatus),
		fmt.Sprintf("🎧 STT Client: %s", sttStatus),
		fmt.Sprintf("🤖 LLM Server: %s", llmServerStatus),
		fmt.Sprintf("<:redis:1429533496954585108> Local Cache: %s", localCacheStatus),
		fmt.Sprintf("<:quickredis:1429533493934948362> Cloud Cache: %s", cloudCacheStatus),
	}, "\n")
}

func PostFinalStatus(s *discordgo.Session, localCache, cloudCache cache.Cache, cfg *config.AllConfig, bootMessageID, cleanupReport string, audioCleanResult, messageCleanResult cache.CleanResult, sttClient interfaces.SpeechToText, stateManager *events.StateManager, logger logger.Logger, systemPrompt string) {
	sysInfo, err := system.GetSysInfo()
	if err != nil {
		logger.Error("Failed to get system info", err)
		sysInfo = &system.SysInfo{}
	}
	cpuUsage, err := system.GetCPUUsage()
	if err != nil {
		logger.Error("Failed to get CPU usage", err)
	}
	memUsage, err := system.GetMemoryUsage()
	if err != nil {
		logger.Error("Failed to get memory usage", err)
	}
	discordStatus := health.GetDiscordStatus(s)
	localCacheStatus := health.GetCacheStatus(localCache, cfg.Cache.Local)
	cloudCacheStatus := health.GetCacheStatus(cloudCache, cfg.Cache.Cloud)
	sttStatus := health.GetSTTStatus(sttClient)
	llmServerStatus := health.GetLLMServerStatus()
	gpuInfo, err := health.GetGPUStatus()
	if err != nil {
		logger.Error("Failed to get GPU status", err)
	}

	storageInfo, err := system.GetStorageInfo()
	if err != nil {
		logger.Error("Failed to get storage info", err)
	}

	systemStatus := formatSystemStatus(sysInfo, cpuUsage, memUsage, gpuInfo)
	storageStatus := formatStorageStatus(storageInfo)
	serviceStatus := formatServiceStatus(discordStatus, sttStatus, llmServerStatus, localCacheStatus, cloudCacheStatus)
	addedMessagesCount, addedMessagesSize := stateManager.GetAddedMessagesStats()
	activeServersStrings := health.GetFormattedActiveServers(s)
	cachedChannelsStrings := health.GetFormattedCachedChannels(s, localCache, logger)
	cachedDMsStrings := health.GetFormattedCachedDMs(s, localCache, logger)
	finalStatus := strings.Join([]string{
		systemStatus,
		"",
		storageStatus,
		"",
		serviceStatus,
		"",
		cleanupReport,
		"",
		"**Essential Tasks**",
		fmt.Sprintf("🗘 Audio Cache: `+%d (%s)` / `-%d (%s)`", 0, humanReadableBytes(0), uint64(audioCleanResult.Count), humanReadableBytes(uint64(audioCleanResult.BytesFreed))),
		fmt.Sprintf("🗘 Message Cache: `+%d (%s)` / `-%d (%s)`", addedMessagesCount, humanReadableBytes(uint64(addedMessagesSize)), messageCleanResult.Count, humanReadableBytes(uint64(messageCleanResult.BytesFreed))),
		"",
		strings.Join(activeServersStrings, "\n"),
		"",
		strings.Join(cachedChannelsStrings, "\n"),
		"",
		strings.Join(cachedDMsStrings, "\n"),
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
