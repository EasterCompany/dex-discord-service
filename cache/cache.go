package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/guild"
	"github.com/bwmarrin/discordgo"
	"github.com/redis/go-redis/v9"
)

const (
	maxMessages = 50
	keyPrefix   = "dex-discord-interface:"
)

// Cache is the interface for our in-memory data store.
type Cache interface {
	AddMessage(key string, m *discordgo.Message) error
	BulkInsertMessages(key string, messages []*discordgo.Message) error
	SaveAudio(key string, data []byte, ttl time.Duration) error
	SaveGuildState(guildID string, state *guild.GuildState) error
	LoadGuildState(guildID string) (*guild.GuildState, error)
	GetAllGuildIDs() ([]string, error)
	CleanAllAudio() (int64, error)
	Ping() error
	Close() error
}

type DB struct {
	rdb *redis.Client
	ctx context.Context
}

func New(cfg *config.ConnectionConfig) (*DB, error) {
	if cfg == nil || cfg.Addr == "" {
		return nil, nil
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Username: cfg.Username,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	ctx := context.Background()
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("could not connect to cache at %s: %w", cfg.Addr, err)
	}
	return &DB{rdb: rdb, ctx: ctx}, nil
}

func (db *DB) Ping() error {
	return db.rdb.Ping(db.ctx).Err()
}

func (db *DB) Close() error {
	return db.rdb.Close()
}

func (db *DB) AddMessage(key string, m *discordgo.Message) error {
	prefixedKey := keyPrefix + key
	jsonMsg, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("could not marshal message: %w", err)
	}
	pipe := db.rdb.Pipeline()
	pipe.LPush(db.ctx, prefixedKey, jsonMsg)
	pipe.LTrim(db.ctx, prefixedKey, 0, maxMessages-1)
	_, err = pipe.Exec(db.ctx)
	return err
}

func (db *DB) BulkInsertMessages(key string, messages []*discordgo.Message) error {
	prefixedKey := keyPrefix + key
	pipe := db.rdb.Pipeline()
	pipe.Del(db.ctx, prefixedKey)
	if len(messages) == 0 {
		_, err := pipe.Exec(db.ctx)
		return err
	}
	msgs := make([]interface{}, len(messages))
	for i, m := range messages {
		jsonMsg, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("could not marshal message %s: %w", m.ID, err)
		}
		msgs[i] = jsonMsg
	}
	pipe.LPush(db.ctx, prefixedKey, msgs...)
	pipe.LTrim(db.ctx, prefixedKey, 0, maxMessages-1)
	_, err := pipe.Exec(db.ctx)
	return err
}

func (db *DB) SaveAudio(key string, data []byte, ttl time.Duration) error {
	prefixedKey := keyPrefix + key
	return db.rdb.Set(db.ctx, prefixedKey, data, ttl).Err()
}

func (db *DB) SaveGuildState(guildID string, state *guild.GuildState) error {
	key := fmt.Sprintf("guild:%s:state", guildID)
	prefixedKey := keyPrefix + key
	jsonState, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("could not marshal guild state: %w", err)
	}
	return db.rdb.Set(db.ctx, prefixedKey, jsonState, 0).Err()
}

func (db *DB) LoadGuildState(guildID string) (*guild.GuildState, error) {
	key := fmt.Sprintf("guild:%s:state", guildID)
	prefixedKey := keyPrefix + key
	jsonState, err := db.rdb.Get(db.ctx, prefixedKey).Result()
	if err != nil {
		if err == redis.Nil {
			return guild.NewGuildState(), nil
		}
		return nil, fmt.Errorf("could not load guild state: %w", err)
	}
	var state guild.GuildState
	if err := json.Unmarshal([]byte(jsonState), &state); err != nil {
		return nil, fmt.Errorf("could not unmarshal guild state: %w", err)
	}
	return &state, nil
}

func (db *DB) GetAllGuildIDs() ([]string, error) {
	var keys []string
	pattern := keyPrefix + "guild:*:state"
	iter := db.rdb.Scan(db.ctx, 0, pattern, 0).Iterator()
	for iter.Next(db.ctx) {
		key := iter.Val()
		trimmedKey := strings.TrimPrefix(key, keyPrefix)
		parts := strings.Split(trimmedKey, ":")
		if len(parts) == 3 {
			keys = append(keys, parts[1])
		}
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}

// CleanAllAudio finds and deletes all audio entries from the cache.
func (db *DB) CleanAllAudio() (int64, error) {
	pattern := keyPrefix + "audio:*"
	var keys []string
	iter := db.rdb.Scan(db.ctx, 0, pattern, 0).Iterator()
	for iter.Next(db.ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return 0, err
	}
	if len(keys) == 0 {
		return 0, nil
	}
	return db.rdb.Del(db.ctx, keys...).Result()
}
