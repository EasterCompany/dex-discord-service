package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/redis/go-redis/v9"
)

// ServiceEntry represents a single service in the service map
type ServiceEntry struct {
	ID          string            `json:"id"`
	Domain      string            `json:"domain,omitempty"`
	Port        string            `json:"port,omitempty"`
	Credentials *RedisCredentials `json:"credentials,omitempty"`
}

// RedisCredentials holds Redis authentication details
type RedisCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

// ServiceMapConfig represents the structure of service-map.json
type ServiceMapConfig struct {
	Services map[string][]ServiceEntry `json:"services"`
}

// GetRedisClient creates and returns a Redis client for local-cache-0
func GetRedisClient(ctx context.Context) (*redis.Client, error) {
	// Load service map to get Redis connection details
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	serviceMapPath := filepath.Join(home, "Dexter", "service-map.json")
	data, err := os.ReadFile(serviceMapPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read service-map.json: %w", err)
	}

	var serviceMap ServiceMapConfig
	if err := json.Unmarshal(data, &serviceMap); err != nil {
		return nil, fmt.Errorf("failed to parse service-map.json: %w", err)
	}

	// Find local-cache-0 in os services
	var cacheDef *ServiceEntry
	if osServices, ok := serviceMap.Services["os"]; ok {
		for i := range osServices {
			if osServices[i].ID == "local-cache-0" {
				cacheDef = &osServices[i]
				break
			}
		}
	}

	if cacheDef == nil {
		return nil, fmt.Errorf("local-cache-0 service not found in service-map.json")
	}

	// Create Redis client options
	opts := &redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cacheDef.Domain, cacheDef.Port),
		Password: "",
		DB:       0,
	}

	if cacheDef.Credentials != nil {
		opts.Password = cacheDef.Credentials.Password
		opts.DB = cacheDef.Credentials.DB
	}

	// Create and test client connection
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping Redis at %s: %w", opts.Addr, err)
	}

	return client, nil
}
