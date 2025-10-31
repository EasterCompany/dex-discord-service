// Package health provides functions for checking the status of various application components.
package health

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/interfaces"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/EasterCompany/dex-discord-interface/system"
	"github.com/bwmarrin/discordgo"
)

// GetFormattedCachedDMs returns a slice of strings, each containing the formatted name of a cached DM channel and its message count.
func GetFormattedCachedDMs(s *discordgo.Session, c cache.Cache, logger logger.Logger) []string {
	if c == nil {
		return []string{"**Cached DMs**", "Cache not configured"}
	}
	keys, err := c.GetAllDMChannels()
	if err != nil {
		return []string{"**Cached DMs**", "Error getting keys"}
	}

	if len(keys) == 0 {
		return []string{"**Cached DMs**", "No DMs cached"}
	}

	dmStrings := []string{"**Cached DMs**"}
	for _, key := range keys {
		ch, err := s.Channel(key)
		if err != nil {
			continue
		}

		name := ch.Name
		if ch.Type == discordgo.ChannelTypeDM && len(ch.Recipients) > 0 {
			name = ch.Recipients[0].Username
		}

		count, err := c.GetMessageCount(fmt.Sprintf("dex-discord-interface:messages:dm:%s", key))
		if err != nil {
			continue
		}

		dmStrings = append(dmStrings, fmt.Sprintf("%s (%d)", name, count))
	}
	return dmStrings
}

// GetFormattedCachedChannels returns a slice of strings, each containing the formatted name of a cached channel and its message count.
func GetFormattedCachedChannels(s *discordgo.Session, c cache.Cache, logger logger.Logger) []string {
	if c == nil {
		return []string{"**Cached Channels**", "Cache not configured"}
	}
	keys, err := c.GetAllMessageCacheKeys()
	if err != nil {
		return []string{"**Cached Channels**", "Error getting keys"}
	}

	if len(keys) == 0 {
		return []string{"**Cached Channels**", "No channels cached"}
	}

	channelStrings := []string{"**Cached Channels**"}
	for _, key := range keys {
		if !strings.Contains(key, ":guild:") {
			continue
		}
		parts := strings.Split(key, ":")
		channelID := parts[len(parts)-1]
		ch, err := s.Channel(channelID)
		if err != nil {
			continue
		}

		count, err := c.GetMessageCount(key)
		if err != nil {
			continue
		}

		channelStrings = append(channelStrings, fmt.Sprintf("%s (%d)", ch.Name, count))
	}
	return channelStrings
}

// GetLLMServerStatus checks and returns the status of the LLM server as a formatted string.
func GetLLMServerStatus() string {
	resp, err := http.Get("http://localhost:11434")
	if err != nil {
		return fmt.Sprintf("**ERROR** `%v`", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("**ERROR** `Status: %s`", resp.Status)
	}
	return "**OK**"
}

// GetDiscordStatus checks and returns the status of the Discord connection as a formatted string.
func GetDiscordStatus(s *discordgo.Session) string {
	if s.DataReady {
		return "**OK**"
	}
	if err := s.Open(); err != nil {
		return fmt.Sprintf("**ERROR** `%v`", err)
	}
	return "**OK** (reconnected)"
}

// GetCacheStatus checks and returns the status of a cache connection as a formatted string.
func GetCacheStatus(c cache.Cache, cfg *config.ConnectionConfig) string {
	if cfg == nil || cfg.Addr == "" {
		return "`Not Configured`"
	}
	if c == nil {
		return "**ERROR** `Initialization failed`"
	}
	if err := c.Ping(); err != nil {
		return fmt.Sprintf("**ERROR** `%v`", err)
	}
	return "**OK**"
}

// GetSTTStatus checks and returns the status of the STT client as a formatted string.
func GetSTTStatus(sttClient interfaces.SpeechToText) string {
	if sttClient == nil {
		return "**ERROR** `Initialization failed`"
	}
	// The STT client doesn't have a built-in ping, so we assume it's OK if it initialized.
	return "**OK**"
}

// GetGPUStatus checks and returns the status of the GPU as a formatted string.
func GetGPUStatus() ([]system.GPUInfo, error) {
	if !system.IsNvidiaGPUInstalled() {
		return nil, nil
	}

	info, err := system.GetGPUInfo()
	if err != nil {
		return nil, fmt.Errorf("**ERROR** `%v`", err)
	}
	return info, nil
}

// GetActiveServers returns a map of server names to server IDs.
func GetActiveServers(s *discordgo.Session) map[string]string {
	guilds := make(map[string]string)
	for _, guild := range s.State.Guilds {
		guilds[guild.Name] = guild.ID
	}
	return guilds
}

// GetActiveChannels returns a map of channel names to channel IDs for all text channels.
func GetActiveChannels(s *discordgo.Session) map[string]string {
	channels := make(map[string]string)
	for _, guild := range s.State.Guilds {
		for _, channel := range guild.Channels {
			if channel.Type == discordgo.ChannelTypeGuildText {
				channels[channel.Name] = channel.ID
			}
		}
	}
	return channels
}

// GetActiveConversations returns a map of user names to channel IDs for all private messages.
func GetActiveConversations(s *discordgo.Session) map[string]string {
	conversations := make(map[string]string)
	for _, channel := range s.State.PrivateChannels {
		if channel.Type == discordgo.ChannelTypeDM {
			for _, recipient := range channel.Recipients {
				conversations[recipient.Username] = channel.ID
			}
		}
	}
	return conversations
}

// GetActiveVoiceSessions returns a map of voice channel names to guild names.
func GetActiveVoiceSessions(s *discordgo.Session) map[string]string {
	sessions := make(map[string]string)
	for _, vc := range s.VoiceConnections {
		channel, err := s.Channel(vc.ChannelID)
		if err != nil {
			continue
		}
		guild, err := s.Guild(vc.GuildID)
		if err != nil {
			continue
		}
		sessions[channel.Name] = guild.Name
	}
	return sessions
}

// GetFormattedActiveServers returns a slice of strings, each containing the formatted name of an active server.
func GetFormattedActiveServers(s *discordgo.Session) []string {
	servers := GetActiveServers(s)
	var serverStrings []string
	if len(servers) > 0 {
		serverStrings = append(serverStrings, "**Active Servers**")
		for name := range servers {
			serverStrings = append(serverStrings, fmt.Sprintf("🌐 %s", name))
		}
	}
	return serverStrings
}
