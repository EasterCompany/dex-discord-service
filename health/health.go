package health

import (
	"fmt"
	"net/http"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/interfaces"
	"github.com/EasterCompany/dex-discord-interface/system"
	"github.com/bwmarrin/discordgo"
)

// GetOllamaStatus checks and returns the status of the Ollama server as a formatted string.
func GetOllamaStatus() string {
	resp, err := http.Get("http://localhost:11434")
	if err != nil {
		return fmt.Sprintf("**ERROR**: `%v`", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("**ERROR**: `Status: %s`", resp.Status)
	}
	return "**OK**"
}

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
func GetGPUStatus() ([]system.GPUInfo, error) {
	if !system.IsNvidiaGPUInstalled() {
		return nil, nil
	}

	info, err := system.GetGPUInfo()
	if err != nil {
		return nil, fmt.Errorf("**ERROR**: `%v`", err)
	}
	return info, nil
}

// GetActiveGuilds returns a map of guild names to guild IDs.
func GetActiveGuilds(s *discordgo.Session) map[string]string {
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

// GetFormattedActiveGuilds returns a slice of strings, each containing the formatted name of an active guild.
func GetFormattedActiveGuilds(s *discordgo.Session) []string {
	guilds := GetActiveGuilds(s)
	var guildStrings []string
	if len(guilds) > 0 {
		guildStrings = append(guildStrings, "**Active Guilds**")
		for name := range guilds {
			guildStrings = append(guildStrings, fmt.Sprintf("🌐 %s", name))
		}
	}
	return guildStrings
}
