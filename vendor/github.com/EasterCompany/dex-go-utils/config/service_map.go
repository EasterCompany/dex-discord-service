package config

import (
	"fmt"

	"github.com/EasterCompany/dex-go-utils/network"
)

// ServiceMapConfig represents the structure of service-map.json
type ServiceMapConfig struct {
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
	ID           string              `json:"id"`
	ShortName    string              `json:"short_name,omitempty"`
	Type         string              `json:"type,omitempty"`
	Repo         string              `json:"repo"`
	Source       string              `json:"source"`
	Domain       string              `json:"domain,omitempty"`
	Port         string              `json:"port,omitempty"`
	InternalPort string              `json:"internal_port,omitempty"`
	Credentials  *ServiceCredentials `json:"credentials,omitempty"`
}

// IsHostedLocally checks if the service's domain resolves to a local address.
func (s *ServiceEntry) IsHostedLocally() bool {
	return network.IsAddressLocal(s.Domain)
}

// IsBuildable returns true if the service is a local Go service that can be compiled.
func (s *ServiceEntry) IsBuildable() bool {
	return s.Type == "cs" || s.Type == "co"
}

// GetInternalPort returns the internal port or a default if not set.
func (s *ServiceEntry) GetInternalPort(defaultPort string) string {
	if s.InternalPort != "" {
		return s.InternalPort
	}
	return defaultPort
}

// ServiceCredentials holds connection credentials for services like Redis
type ServiceCredentials struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

// LoadServiceMap loads the service map from the standard Dexter config location
func LoadServiceMap() (*ServiceMapConfig, error) {
	return LoadConfig[ServiceMapConfig]("service-map.json")
}

// SaveServiceMap saves the service map to the standard Dexter config location
func SaveServiceMap(config *ServiceMapConfig) error {
	return SaveConfig("service-map.json", config)
}

// GetServiceURL finds a service by ID and category and returns its full HTTP URL
func (s *ServiceMapConfig) GetServiceURL(id, category, defaultPort string) string {
	for _, entry := range s.Services[category] {
		if entry.ID == id {
			host := entry.Domain
			if host == "" {
				host = "127.0.0.1"
			}
			return fmt.Sprintf("http://%s:%s", host, entry.Port)
		}
	}
	return fmt.Sprintf("http://127.0.0.1:%s", defaultPort)
}

// ResolveHubURL specifically finds the dex-model-service URL
func (s *ServiceMapConfig) ResolveHubURL() string {
	return s.GetServiceURL("dex-model-service", "co", "8400")
}

// ResolveService finds a service by ID across all categories.
func (s *ServiceMapConfig) ResolveService(id string) (*ServiceEntry, error) {
	for _, entries := range s.Services {
		for i := range entries {
			if entries[i].ID == id {
				return &entries[i], nil
			}
		}
	}
	return nil, fmt.Errorf("service %s not found in service-map.json", id)
}

// GetSanitized returns a version of the map with credentials masked (stub for now if needed)
func (c *ServiceMapConfig) GetSanitized() map[string]interface{} {
	sanitized := make(map[string]interface{})
	sanitized["service_types"] = c.ServiceTypes
	sanitized["services"] = c.Services
	return sanitized
}

// MergeDefaults adds missing services and types from the default config.
// Returns true if any changes were made.
func (c *ServiceMapConfig) MergeDefaults(defaults *ServiceMapConfig) bool {
	changed := false

	// Merge ServiceTypes
	if len(c.ServiceTypes) == 0 && len(defaults.ServiceTypes) > 0 {
		c.ServiceTypes = defaults.ServiceTypes
		changed = true
	} else {
		for _, defType := range defaults.ServiceTypes {
			found := false
			for _, cType := range c.ServiceTypes {
				if cType.Type == defType.Type {
					found = true
					break
				}
			}
			if !found {
				c.ServiceTypes = append(c.ServiceTypes, defType)
				changed = true
			}
		}
	}

	// Merge Services
	if c.Services == nil {
		c.Services = make(map[string][]ServiceEntry)
		changed = true
	}

	for category, defServices := range defaults.Services {
		if _, exists := c.Services[category]; !exists {
			c.Services[category] = defServices
			changed = true
			continue
		}

		for _, defSvc := range defServices {
			found := false
			for i, cSvc := range c.Services[category] {
				if cSvc.ID == defSvc.ID {
					found = true
					// Heal missing fields
					if cSvc.ShortName == "" && defSvc.ShortName != "" {
						c.Services[category][i].ShortName = defSvc.ShortName
						changed = true
					}
					if cSvc.Type == "" && defSvc.Type != "" {
						c.Services[category][i].Type = defSvc.Type
						changed = true
					}
					if cSvc.Repo == "" && defSvc.Repo != "" {
						c.Services[category][i].Repo = defSvc.Repo
						changed = true
					}
					if cSvc.Source == "" && defSvc.Source != "" {
						c.Services[category][i].Source = defSvc.Source
						changed = true
					}
					break
				}
			}
			if !found {
				c.Services[category] = append(c.Services[category], defSvc)
				changed = true
			}
		}
	}

	return changed
}
