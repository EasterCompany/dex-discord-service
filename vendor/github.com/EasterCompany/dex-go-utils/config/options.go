package config

import (
	"encoding/json"
)

// OptionsConfig represents the structure of options.json
type OptionsConfig struct {
	Doc string `json:"_doc"`

	// Compatibility fields for commonly used services (now typed)
	Discord    DiscordOptions    `json:"-"`
	Fabricator FabricatorOptions `json:"-"`

	// Services maps all ServiceIDs to their specific configuration
	Services map[string]ServiceConfig `json:"-"`
}

// ServiceConfig holds service-specific settings
type ServiceConfig struct {
	Options map[string]interface{} `json:"options"`
}

// Typed Option Structs

type FabricatorOptions struct {
	OAuthClientID     string `json:"oauth_client_id"`
	OAuthClientSecret string `json:"oauth_client_secret"`
	GCPProjectID      string `json:"gcp_project_id"`
}

type CognitiveOptions struct {
}

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

type RoleConfig struct {
	Admin       string `json:"admin"`
	Moderator   string `json:"moderator"`
	Contributor string `json:"contributor"`
	User        string `json:"user"`
}

// LoadOptions loads the options from the standard Dexter config location
func LoadOptions() (*OptionsConfig, error) {
	return LoadConfig[OptionsConfig]("options.json")
}

// MarshalJSON customizes the JSON output to be flat at the root
func (c OptionsConfig) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{})
	if c.Doc != "" {
		m["_doc"] = c.Doc
	}

	// Add all services from the map
	for k, v := range c.Services {
		m[k] = v
	}

	// Ensure explicitly defined fields are also in the map/output if populated
	if c.Discord.Token != "" {
		m["dex-discord-service"] = ServiceConfig{Options: ToMap(c.Discord)}
	}
	if c.Fabricator.OAuthClientID != "" {
		m["dex-fabricator-cli"] = ServiceConfig{Options: ToMap(c.Fabricator)}
	}

	return json.MarshalIndent(m, "", "  ")
}

// UnmarshalJSON handles the flat root structure
func (c *OptionsConfig) UnmarshalJSON(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	c.Services = make(map[string]ServiceConfig)
	for k, v := range m {
		if k == "_doc" {
			if s, ok := v.(string); ok {
				c.Doc = s
			}
			continue
		}

		// Check if it's a service configuration (contains "options" key)
		if svcMap, ok := v.(map[string]interface{}); ok {
			if opts, ok := svcMap["options"].(map[string]interface{}); ok {
				svcConf := ServiceConfig{Options: opts}
				c.Services[k] = svcConf

				// Populate compatibility fields
				if k == "dex-discord-service" {
					jsonData, _ := json.Marshal(opts)
					json.Unmarshal(jsonData, &c.Discord)
				} else if k == "dex-fabricator-cli" {
					jsonData, _ := json.Marshal(opts)
					json.Unmarshal(jsonData, &c.Fabricator)
				}
			}
		}
	}
	return nil
}

// SaveOptions saves the options.json file to the standard Dexter config location
func SaveOptions(options *OptionsConfig) error {
	return SaveConfig("options.json", options)
}

// MergeDefaults merges default options into the user config.
func (c *OptionsConfig) MergeDefaults(defaults *OptionsConfig) bool {
	changed := false

	if c.Doc == "" && defaults.Doc != "" {
		c.Doc = defaults.Doc
		changed = true
	}

	if c.Services == nil {
		c.Services = make(map[string]ServiceConfig)
		changed = true
	}

	for svcID, defSvcConf := range defaults.Services {
		userSvcConf, exists := c.Services[svcID]
		if !exists {
			c.Services[svcID] = defSvcConf
			changed = true
			continue
		}

		if userSvcConf.Options == nil {
			userSvcConf.Options = make(map[string]interface{})
			changed = true
		}

		for k, v := range defSvcConf.Options {
			if _, ok := userSvcConf.Options[k]; !ok {
				userSvcConf.Options[k] = v
				changed = true
			}
		}
		c.Services[svcID] = userSvcConf
	}

	// Sync compatibility fields
	if opts := c.GetServiceOptions("dex-discord-service"); opts != nil {
		jsonData, _ := json.Marshal(opts)
		json.Unmarshal(jsonData, &c.Discord)
	}
	if opts := c.GetServiceOptions("dex-fabricator-cli"); opts != nil {
		jsonData, _ := json.Marshal(opts)
		json.Unmarshal(jsonData, &c.Fabricator)
	}

	return changed
}

// GetServiceOptions returns the options for a specific service
func (c *OptionsConfig) GetServiceOptions(serviceID string) map[string]interface{} {
	if c.Services == nil {
		return nil
	}
	if svc, ok := c.Services[serviceID]; ok {
		return svc.Options
	}
	return nil
}

// SetServiceOption sets a specific option for a service
func (c *OptionsConfig) SetServiceOption(serviceID, key string, value interface{}) {
	if c.Services == nil {
		c.Services = make(map[string]ServiceConfig)
	}
	svc, ok := c.Services[serviceID]
	if !ok {
		svc = ServiceConfig{Options: make(map[string]interface{})}
	}
	if svc.Options == nil {
		svc.Options = make(map[string]interface{})
	}
	svc.Options[key] = value
	c.Services[serviceID] = svc

	// Sync compatibility fields
	if serviceID == "dex-discord-service" {
		jsonData, _ := json.Marshal(svc.Options)
		json.Unmarshal(jsonData, &c.Discord)
	} else if serviceID == "dex-fabricator-cli" {
		jsonData, _ := json.Marshal(svc.Options)
		json.Unmarshal(jsonData, &c.Fabricator)
	}
}

// ToMap helper
func ToMap(v interface{}) map[string]interface{} {
	data, _ := json.Marshal(v)
	var res map[string]interface{}
	json.Unmarshal(data, &res)
	return res
}
