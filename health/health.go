package health

import (
	"fmt"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/interfaces"
	"github.com/EasterCompany/dex-discord-interface/system"
	"github.com/bwmarrin/discordgo"
)

// GetDiscordStatus checks and returns the status of the Discord connection as a formatted string.
func GetDiscordStatus(s *discordgo.Session) string {
	if s.DataReady {
		return "**OK**"
	}
	if err := s.Open(); err != nil {
		return fmt.Sprintf("**ERROR**: `%v`", err)
	}
	return "**OK** (reconnected)"
}

// GetCacheStatus checks and returns the status of a cache connection as a formatted string.
func GetCacheStatus(c cache.Cache, cfg *config.ConnectionConfig) string {
	if cfg == nil || cfg.Addr == "" {
		return "`Not Configured`"
	}
	if c == nil {
		return "**ERROR**: `Initialization failed`"
	}
	if err := c.Ping(); err != nil {
		return fmt.Sprintf("**ERROR**: `%v`", err)
	}
	return "**OK**"
}

// GetSTTStatus checks and returns the status of the STT client as a formatted string.
func GetSTTStatus(sttClient interfaces.SpeechToText) string {
	if sttClient == nil {
		return "**ERROR**: `Initialization failed`"
	}
	// The STT client doesn't have a built-in ping, so we assume it's OK if it initialized.
	return "**OK**"
}

// GetGPUStatus checks and returns the status of the GPU as a formatted string.
func GetGPUStatus() (string, *system.GPUInfo) {
	if !system.IsNvidiaGPUInstalled() {
		return "", nil
	}

	info, err := system.GetGPUInfo()
	if err != nil {
		return fmt.Sprintf("**ERROR**: `%v`", err), nil
	}
	return "**OK**", info
}
