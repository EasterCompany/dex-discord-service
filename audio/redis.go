package audio

import (
	"context"
	"fmt"

	"github.com/EasterCompany/dex-discord-service/config"
	"github.com/EasterCompany/dex-go-utils/utils"
	"github.com/redis/go-redis/v9"
)

// GetRedisClient creates and returns a Redis client for local-cache-0.
// It now uses the shared implementation from dex-go-utils.
func GetRedisClient(ctx context.Context) (*redis.Client, error) {
	serviceMap, err := config.LoadServiceMap()
	if err != nil {
		return nil, fmt.Errorf("failed to load service map: %w", err)
	}

	return utils.GetRedisClient(ctx, serviceMap)
}
