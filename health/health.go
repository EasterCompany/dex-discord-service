package health

import (
	"fmt"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/config"
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
