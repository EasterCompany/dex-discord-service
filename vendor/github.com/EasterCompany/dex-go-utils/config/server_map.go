package config

import (
	"fmt"
)

// ServerMapConfig represents the structure of server-map.json
type ServerMapConfig struct {
	Servers map[string]Server `json:"servers"`
}

// Server represents a single server in the server map
type Server struct {
	User        string   `json:"user"`
	Key         string   `json:"key"`
	PublicIPV4  string   `json:"public_ipv4"`
	PrivateIPV4 string   `json:"private_ipv4"`
	PublicIPV6  string   `json:"public_ipv6"`
	Services    []string `json:"services,omitempty"`
}

// LoadServerMap loads the server map from the standard Dexter config location.
func LoadServerMap() (*ServerMapConfig, error) {
	return LoadConfig[ServerMapConfig]("server-map.json")
}

// SaveServerMap saves the server map to the standard Dexter config location.
func SaveServerMap(config *ServerMapConfig) error {
	return SaveConfig("server-map.json", config)
}

// GetServerForHost finds the server configuration for a given host (IP or hostname).
func (sm *ServerMapConfig) GetServerForHost(host string) (*Server, error) {
	// 1. Direct key match
	if server, ok := sm.Servers[host]; ok {
		return &server, nil
	}

	// 2. IP match
	for _, server := range sm.Servers {
		if server.PublicIPV4 == host || server.PrivateIPV4 == host || server.PublicIPV6 == host {
			return &server, nil
		}
	}

	return nil, fmt.Errorf("server config not found for host: %s", host)
}

// GetServerForService finds the server name and configuration for a given service ID.
func (sm *ServerMapConfig) GetServerForService(serviceID string) (string, *Server) {
	for name, server := range sm.Servers {
		for _, sID := range server.Services {
			if sID == serviceID {
				return name, &server
			}
		}
	}
	return "", nil
}
