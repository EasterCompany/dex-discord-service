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

const keyPrefix = "dex-discord-interface:"

type DB struct {
	rdb *redis.Client
	ctx context.Context
}

type Cache interface {
	Ping() error
	SaveMessage(key string, m *discordgo.Message) error
	BulkInsertMessages(key string, messages []*discordgo.Message) error
	AddMessage(key string, message *discordgo.Message) error
	SaveGuildState(guildID string, state *guild.GuildState) error
	LoadGuildState(guildID string) (*guild.GuildState, error)
	GetAllGuildIDs() ([]string, error)
	SaveAudio(key string, data []byte, ttl time.Duration) error
	CleanAllAudio() (CleanResult, error)
	CleanAllMessages() (CleanResult, error)
}

type CleanResult struct {
	Count      int
	BytesFreed int64
}

func New(cfg *config.ConnectionConfig) (Cache, error) {
	if cfg == nil || cfg.Addr == "" {
		return nil, nil // Not configured, no error
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		Username: cfg.Username,
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

func (db *DB) prefixedKey(key string) string {
	return keyPrefix + key
}

func (db *DB) SaveMessage(key string, m *discordgo.Message) error {
	jsonMsg, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("could not marshal message: %w", err)
	}
	return db.rdb.RPush(db.ctx, db.prefixedKey(key), jsonMsg).Err()
}

func (db *DB) BulkInsertMessages(key string, messages []*discordgo.Message) error {
	pipe := db.rdb.Pipeline()
	prefixedKey := db.prefixedKey(key)
	pipe.Del(db.ctx, prefixedKey)
	for _, m := range messages {
		jsonMsg, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("could not marshal message: %w", err)
		}
		pipe.RPush(db.ctx, prefixedKey, jsonMsg)
	}
	pipe.LTrim(db.ctx, prefixedKey, -50, -1)
	_, err := pipe.Exec(db.ctx)
	return err
}

func (db *DB) AddMessage(key string, message *discordgo.Message) error {
	pipe := db.rdb.Pipeline()
	prefixedKey := db.prefixedKey(key)
	jsonMsg, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("could not marshal message: %w", err)
	}
	pipe.RPush(db.ctx, prefixedKey, jsonMsg)
	pipe.LTrim(db.ctx, prefixedKey, -50, -1)
	_, err = pipe.Exec(db.ctx)
	return err
}

func (db *DB) SaveGuildState(guildID string, state *guild.GuildState) error {
	key := db.prefixedKey(fmt.Sprintf("guild:%s:state", guildID))
	jsonState, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("could not marshal guild state: %w", err)
	}
	return db.rdb.Set(db.ctx, key, jsonState, 0).Err()
}

func (db *DB) LoadGuildState(guildID string) (*guild.GuildState, error) {
	key := db.prefixedKey(fmt.Sprintf("guild:%s:state", guildID))
	jsonState, err := db.rdb.Get(db.ctx, key).Result()
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
	state.ActiveStreams = make(map[uint32]*guild.UserStream)
	return &state, nil
}

func (db *DB) GetAllGuildIDs() ([]string, error) {
	var keys []string
	pattern := db.prefixedKey("guild:*:state")
	iter := db.rdb.Scan(db.ctx, 0, pattern, 0).Iterator()
	for iter.Next(db.ctx) {
		key := strings.TrimPrefix(iter.Val(), db.prefixedKey("guild:"))
		guildID := strings.Split(key, ":")[0]
		keys = append(keys, guildID)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}

func (db *DB) SaveAudio(key string, data []byte, ttl time.Duration) error {
	return db.rdb.Set(db.ctx, db.prefixedKey(key), data, ttl).Err()
}

func (db *DB) cleanKeysByPattern(pattern string) (CleanResult, error) {
	var result CleanResult
	var keysToDelete []string
	iter := db.rdb.Scan(db.ctx, 0, db.prefixedKey(pattern), 0).Iterator()
	for iter.Next(db.ctx) {
		keysToDelete = append(keysToDelete, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return result, err
	}
	if len(keysToDelete) == 0 {
		return result, nil
	}
	pipe := db.rdb.Pipeline()
	for _, key := range keysToDelete {
		// For simplicity and performance, we'll get the size of string values.
		// For lists (messages), this will be less accurate but still a good indicator.
		val, err := db.rdb.Get(db.ctx, key).Result()
		if err == nil {
			result.BytesFreed += int64(len(val))
		}
		pipe.Del(db.ctx, key)
	}
	_, err := pipe.Exec(db.ctx)
	if err != nil {
		return result, err
	}
	result.Count = len(keysToDelete)
	return result, nil
}

func (db *DB) CleanAllAudio() (CleanResult, error) {
	return db.cleanKeysByPattern("audio:*")
}

func (db *DB) CleanAllMessages() (CleanResult, error) {
	return db.cleanKeysByPattern("messages:*")
}
