package config

import (
	sharedConfig "github.com/EasterCompany/dex-go-utils/config"
)

// Aliases to shared types in dex-go-utils
type ServiceMapConfig = sharedConfig.ServiceMapConfig
type ServiceEntry = sharedConfig.ServiceEntry
type ServiceCredentials = sharedConfig.ServiceCredentials
type OptionsConfig = sharedConfig.OptionsConfig
type SystemConfig = sharedConfig.SystemConfig

// DiscordOptions holds Discord-specific settings
type DiscordOptions struct {
	Token               string     `json:"token"`
	ServerID            string     `json:"server_id"`
	DebugChannelID      string     `json:"debug_channel_id"`
	BuildChannelID      string     `json:"build_channel_id"`
	MasterUser          string     `json:"master_user"`
	DefaultVoiceChannel string     `json:"default_voice_channel"`
	QuietMode           bool       `json:"quiet_mode"`
	Roles               RoleConfig `json:"roles"`
}

// RoleConfig holds role ID mapping
type RoleConfig struct {
	Admin       string `json:"admin"`
	Moderator   string `json:"moderator"`
	Contributor string `json:"contributor"`
	User        string `json:"user"`
}
