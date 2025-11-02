package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Hardcoded Dexter environment layout
const (
	DexterRoot        = "~/Dexter"
	EasterCompanyRoot = "~/EasterCompany"
	ServiceID         = "dex-discord-service"
)

// Config holds all application configuration
type Config struct {
	DiscordToken  string `json:"discord_token"`
	ServerID      string `json:"server_id"`
	LogChannelID  string `json:"log_channel_id"`
	RedisAddr     string `json:"redis_addr"`
	RedisPassword string `json:"redis_password"`
	RedisDB       int    `json:"redis_db"`

	// Service configuration
	ServiceAddr string `json:"service_addr"` // IP address/domain for this service
	ServicePort int    `json:"service_port"` // HTTP port for /status endpoint (default: 8300)

	// External service endpoints
	Services map[string]string `json:"services"` // service_name -> base_url (without /status)

	// Command permissions
	CommandPermissions CommandPermissions `json:"command_permissions"`

	// Server persona - defines bot behavior and personality
	ServerPersona ServerPersona `json:"server_persona"`
}

// CommandPermissions defines who can issue commands
type CommandPermissions struct {
	// DefaultLevel: 0 = server owner only, 1 = server owner and allowed roles, 2 = everyone
	DefaultLevel int `json:"default_level"`

	// AllowedRoles: list of role IDs that can issue commands (only applies when DefaultLevel = 1)
	AllowedRoles []string `json:"allowed_roles"`

	// UserWhitelist: user IDs that can always issue commands regardless of other settings (angels)
	UserWhitelist []string `json:"user_whitelist"`
}

// ServerPersona defines the bot's personality and behavior rules for this server
type ServerPersona struct {
	// Name: the bot's persona name (e.g., "Helpful Assistant", "Sarcastic Friend")
	Name string `json:"name"`

	// Personality: description of the bot's personality traits
	Personality string `json:"personality"`

	// BehaviorRules: list of specific behavior rules the bot should follow
	BehaviorRules []string `json:"behavior_rules"`

	// ResponseStyle: how the bot should structure responses (e.g., "concise", "detailed", "casual")
	ResponseStyle string `json:"response_style"`

	// SystemPrompt: optional custom system prompt override for LLM integration
	SystemPrompt string `json:"system_prompt"`

	// Enabled: whether to use this persona configuration
	Enabled bool `json:"enabled"`
}

// Centralized config structures from ~/Dexter/config/

// OptionsConfig represents the structure of options.json
type OptionsConfig struct {
	Python struct {
		Version float64 `json:"version"`
		Venv    string  `json:"venv"`
		Bin     string  `json:"bin"`
		Pip     string  `json:"pip"`
	} `json:"python"`
	Discord struct {
		Token          string `json:"token"`
		ServerID       string `json:"server_id"`
		DebugChannelID string `json:"debug_channel_id"`
	} `json:"discord"`
	Redis struct {
		Password string `json:"password"`
		DB       int    `json:"db"`
	} `json:"redis"`
	CommandPermissions CommandPermissions `json:"command_permissions"`
}

// ServiceMapConfig represents the structure of service-map.json
type ServiceMapConfig struct {
	Doc          string                    `json:"_doc"`
	ServiceTypes []ServiceType             `json:"service_types"`
	Services     map[string][]ServiceEntry `json:"services"`
}

// ServiceType defines a category of services
type ServiceType struct {
	Type        string `json:"type"`
	Label       string `json:"label"`
	Description string `json:"description"`
	MinPort     int    `json:"min_port"`
	MaxPort     int    `json:"max_port"`
}

// ServiceEntry represents a single service in the service map
type ServiceEntry struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Repo   string `json:"repo"`
	Addr   string `json:"addr"`
	Socket string `json:"socket"`
}

// expandPath expands ~ to the user's home directory
func expandPath(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, path[2:]), nil
}

// normalizePath converts absolute paths with /home/<user> to ~ notation
func normalizePath(path string) (string, error) {
	if path == "" {
		return path, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path, err
	}

	// If path starts with /home/<user>, replace with ~
	if strings.HasPrefix(path, homeDir) {
		return "~" + strings.TrimPrefix(path, homeDir), nil
	}

	return path, nil
}

// healServiceMap checks and fixes service-map.json for hardcoded user paths
func healServiceMap(serviceMap *ServiceMapConfig) (bool, error) {
	healed := false

	for serviceType, serviceList := range serviceMap.Services {
		for i := range serviceList {
			svc := &serviceList[i]

			// Normalize the source path
			normalizedSource, err := normalizePath(svc.Source)
			if err == nil && normalizedSource != svc.Source {
				svc.Source = normalizedSource
				healed = true
			}
		}
		serviceMap.Services[serviceType] = serviceList
	}

	return healed, nil
}

// saveServiceMap writes the healed service-map.json back to disk
func saveServiceMap(serviceMap *ServiceMapConfig, path string) error {
	data, err := json.MarshalIndent(serviceMap, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal service map: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write service map: %w", err)
	}

	return nil
}

// Load reads configuration from centralized ~/Dexter/config/ files
func Load() (*Config, error) {
	// Load options.json
	optionsPath, err := expandPath(filepath.Join(DexterRoot, "config", "options.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to expand options path: %w", err)
	}

	optionsData, err := os.ReadFile(optionsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read options.json: %w", err)
	}

	var options OptionsConfig
	if err := json.Unmarshal(optionsData, &options); err != nil {
		return nil, fmt.Errorf("failed to parse options.json: %w", err)
	}

	// Load service-map.json
	serviceMapPath, err := expandPath(filepath.Join(DexterRoot, "config", "service-map.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to expand service-map path: %w", err)
	}

	serviceMapData, err := os.ReadFile(serviceMapPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read service-map.json: %w", err)
	}

	var serviceMap ServiceMapConfig
	if err := json.Unmarshal(serviceMapData, &serviceMap); err != nil {
		return nil, fmt.Errorf("failed to parse service-map.json: %w", err)
	}

	// Heal service map if needed (fix hardcoded user paths)
	healed, err := healServiceMap(&serviceMap)
	if err != nil {
		return nil, fmt.Errorf("failed to heal service map: %w", err)
	}
	if healed {
		// Save the healed config back to disk
		if err := saveServiceMap(&serviceMap, serviceMapPath); err != nil {
			return nil, fmt.Errorf("failed to save healed service map: %w", err)
		}
		fmt.Println("Config healing: Fixed hardcoded user paths in service-map.json")
	}

	// Find this service in the service map
	var thisService *ServiceEntry
	for _, services := range serviceMap.Services {
		for i := range services {
			if services[i].ID == ServiceID {
				thisService = &services[i]
				break
			}
		}
		if thisService != nil {
			break
		}
	}

	if thisService == nil {
		return nil, fmt.Errorf("service %s not found in service-map.json", ServiceID)
	}

	// Extract port from this service's addr
	servicePort := 8300 // Default third-party integration port
	// Parse port from addr if needed (thisService.Addr contains the port)

	// Build services map from service-map.json
	services := make(map[string]string)
	for _, serviceList := range serviceMap.Services {
		for _, svc := range serviceList {
			if svc.ID != ServiceID { // Don't include self
				// Remove trailing slash for consistency
				addr := strings.TrimSuffix(svc.Addr, "/")
				services[svc.ID] = addr
			}
		}
	}

	// Build the final config
	cfg := &Config{
		DiscordToken:       options.Discord.Token,
		ServerID:           options.Discord.ServerID,
		LogChannelID:       options.Discord.DebugChannelID,
		RedisAddr:          "127.0.0.1:6379", // Default Redis address
		RedisPassword:      options.Redis.Password,
		RedisDB:            options.Redis.DB,
		ServiceAddr:        "127.0.0.1",
		ServicePort:        servicePort,
		Services:           services,
		CommandPermissions: options.CommandPermissions,
	}

	// Validate required fields
	if cfg.DiscordToken == "" {
		return nil, fmt.Errorf("discord token is required in options.json")
	}
	if cfg.ServerID == "" {
		return nil, fmt.Errorf("server_id is required in options.json")
	}
	if cfg.LogChannelID == "" {
		return nil, fmt.Errorf("debug_channel_id is required in options.json")
	}

	// Set command permission defaults
	if cfg.CommandPermissions.AllowedRoles == nil {
		cfg.CommandPermissions.AllowedRoles = []string{}
	}
	if cfg.CommandPermissions.UserWhitelist == nil {
		cfg.CommandPermissions.UserWhitelist = []string{}
	}

	// Set server persona defaults
	if cfg.ServerPersona.BehaviorRules == nil {
		cfg.ServerPersona.BehaviorRules = []string{}
	}
	if cfg.ServerPersona.Name == "" && cfg.ServerPersona.Enabled {
		cfg.ServerPersona.Name = "Dexter"
	}

	return cfg, nil
}

// GetServiceStatusURL returns the full status endpoint URL for a given service
func (c *Config) GetServiceStatusURL(serviceName string) string {
	baseURL, exists := c.Services[serviceName]
	if !exists {
		return ""
	}
	// Remove trailing slash if present
	if len(baseURL) > 0 && baseURL[len(baseURL)-1] == '/' {
		return baseURL + "status"
	}
	return baseURL + "/status"
}

// GetAllServiceStatusURLs returns a map of service names to their status endpoint URLs
func (c *Config) GetAllServiceStatusURLs() map[string]string {
	statusURLs := make(map[string]string, len(c.Services))
	for serviceName := range c.Services {
		statusURLs[serviceName] = c.GetServiceStatusURL(serviceName)
	}
	return statusURLs
}

// GetServiceBaseURL returns the base URL for a given service
func (c *Config) GetServiceBaseURL(serviceName string) string {
	return c.Services[serviceName]
}
