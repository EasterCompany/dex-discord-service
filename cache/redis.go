package cache

import (
	"context"
	"fmt"

	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/go-redis/redis/v8"
)

const (
	// Redis keys
	MessagesKey = "dexter:discord:messages"
	EventsKey   = "dexter:discord:events"

	// Pub/Sub channels
	EventStreamChannel = "dexter:events"
)

// RedisClient wraps the go-redis client
type RedisClient struct {
	*redis.Client
}

// NewRedisClient creates and configures a new Redis client
func NewRedisClient(cfg *config.Config) (*RedisClient, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	// Verify connection
	if _, err := rdb.Ping(context.Background()).Result(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisClient{rdb}, nil
}

// AddToList adds an item to the start of a list and trims the list to a max length.
func (c *RedisClient) AddToList(ctx context.Context, key, value string, maxLength int64) error {
	pipe := c.Pipeline()
	pipe.LPush(ctx, key, value)
	pipe.LTrim(ctx, key, 0, maxLength-1)
	_, err := pipe.Exec(ctx)
	return err
}

// GetListRange returns a range of items from a list.
func (c *RedisClient) GetListRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.LRange(ctx, key, start, stop).Result()
}

// PublishEvent publishes an event to the event stream.
func (c *RedisClient) PublishEvent(ctx context.Context, channel, message string) error {
	return c.Publish(ctx, channel, message).Err()
}

// ClearCache deletes all keys in the current database.
func (c *RedisClient) ClearCache(ctx context.Context) error {
	return c.FlushDB(ctx).Err()
}
