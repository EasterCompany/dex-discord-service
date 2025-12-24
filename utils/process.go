package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// ReportProcess updates a process's state in Redis.
func ReportProcess(ctx context.Context, redisClient *redis.Client, processID string, state string) {
	if redisClient == nil {
		return
	}
	key := fmt.Sprintf("process:info:%s", processID)
	data := map[string]interface{}{
		"channel_id": processID, // Legacy field name for dashboard compatibility
		"state":      state,
		"retries":    0,
		"start_time": time.Now().Unix(),
		"pid":        os.Getpid(),
		"updated_at": time.Now().Unix(),
	}

	jsonBytes, _ := json.Marshal(data)
	redisClient.Set(ctx, key, jsonBytes, 0)
}

// ClearProcess removes a process from Redis.
func ClearProcess(ctx context.Context, redisClient *redis.Client, processID string) {
	if redisClient == nil {
		return
	}
	key := fmt.Sprintf("process:info:%s", processID)
	redisClient.Del(ctx, key)
}
