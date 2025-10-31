// Package cache provides an interface for interacting with the Redis cache.
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
	GetLastNMessages(key string, n int64) ([]*discordgo.Message, error)
	SaveGuildState(guildID string, state *guild.GuildState) error
	LoadGuildState(guildID string) (*guild.GuildState, error)
	GetAllGuildIDs() ([]string, error)
	GetAllMessageCacheKeys() ([]string, error)
	GetMessageCount(key string) (int64, error)
	AddDMChannel(channelID string) error
	GetAllDMChannels() ([]string, error)
	SaveAudio(key string, data []byte, ttl time.Duration) error
	GetAudio(key string) ([]byte, error)
	DeleteAudio(key string) error
	CleanAllAudio() (CleanResult, error)
	CleanAllMessages() (CleanResult, error)
	GenerateMessageCacheKey(guildID, channelID string) string
	GenerateAudioCacheKey(filename string) string
	GenerateGuildStateKey(guildID string) string
}

// ... (existing code)

func (db *DB) GenerateMessageCacheKey(guildID, channelID string) string {
	if guildID == "" {
		return db.prefixedKey(fmt.Sprintf("messages:dm:%s", channelID))
	}
	return db.prefixedKey(fmt.Sprintf("messages:guild:%s:channel:%s", guildID, channelID))
}

func (db *DB) GenerateAudioCacheKey(filename string) string {
	return db.prefixedKey(fmt.Sprintf("audio:%s", filename))
}

func (db *DB) GenerateGuildStateKey(guildID string) string {
	return db.prefixedKey(fmt.Sprintf("guild:%s:state", guildID))
}

type DebugCache interface {
	GetAllKeys() ([]string, error)
	GetType(key string) (string, error)
	Get(key string) (string, error)
	LRange(key string, start, stop int64) ([]string, error)
}

type CleanResult struct {
	Count      int
	BytesFreed int64
}

func NewDebug(cfg *config.ConnectionConfig) (DebugCache, error) {
	if cfg == nil || cfg.Addr == "" {
		return nil, nil
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

func New(cfg *config.ConnectionConfig) (Cache, error) {
	if cfg == nil || cfg.Addr == "" {
		return nil, nil
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
	prefixedKey := db.prefixedKey(key)

	jsonMsgs := make([]interface{}, len(messages))
	for i, m := range messages {
		jsonMsg, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("could not marshal message: %w", err)
		}
		jsonMsgs[i] = jsonMsg
	}

	_, err := db.rdb.TxPipelined(db.ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(db.ctx, prefixedKey)
		if len(jsonMsgs) > 0 {
			pipe.RPush(db.ctx, prefixedKey, jsonMsgs...)
		}
		pipe.LTrim(db.ctx, prefixedKey, -50, -1)
		return nil
	})

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

func (db *DB) GetLastNMessages(key string, n int64) ([]*discordgo.Message, error) {
	prefixedKey := db.prefixedKey(key)
	results, err := db.rdb.LRange(db.ctx, prefixedKey, -n, -1).Result()
	if err != nil {
		if err == redis.Nil {
			return []*discordgo.Message{}, nil
		}
		return nil, fmt.Errorf("could not get messages from cache: %w", err)
	}

	messages := make([]*discordgo.Message, 0, len(results))
	for _, res := range results {
		var msg discordgo.Message
		if err := json.Unmarshal([]byte(res), &msg); err != nil {
			return nil, fmt.Errorf("could not unmarshal message from cache: %w", err)
		}
		messages = append(messages, &msg)
	}
	return messages, nil
}

func (db *DB) SaveGuildState(guildID string, state *guild.GuildState) error {
	key := db.GenerateGuildStateKey(guildID)
	jsonState, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("could not marshal guild state: %w", err)
	}
	return db.rdb.Set(db.ctx, key, jsonState, 0).Err()
}

func (db *DB) LoadGuildState(guildID string) (*guild.GuildState, error) {
	key := db.GenerateGuildStateKey(guildID)
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

func (db *DB) GetAllMessageCacheKeys() ([]string, error) {
	var keys []string
	pattern := db.prefixedKey("messages:*")
	iter := db.rdb.Scan(db.ctx, 0, pattern, 0).Iterator()
	for iter.Next(db.ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}

func (db *DB) GetMessageCount(key string) (int64, error) {
	return db.rdb.LLen(db.ctx, key).Result()
}

func (db *DB) AddDMChannel(channelID string) error {
	return db.rdb.SAdd(db.ctx, db.prefixedKey("dm_channels"), channelID).Err()
}

func (db *DB) GetAllDMChannels() ([]string, error) {
	return db.rdb.SMembers(db.ctx, db.prefixedKey("dm_channels")).Result()
}

func (db *DB) SaveAudio(key string, data []byte, ttl time.Duration) error {
	return db.rdb.Set(db.ctx, db.prefixedKey(key), data, ttl).Err()
}

func (db *DB) GetAudio(key string) ([]byte, error) {
	return db.rdb.Get(db.ctx, db.prefixedKey(key)).Bytes()
}

func (db *DB) DeleteAudio(key string) error {
	return db.rdb.Del(db.ctx, db.prefixedKey(key)).Err()
}

func (db *DB) cleanKeysByPattern(pattern string) (CleanResult, error) {
	var result CleanResult
	pipe := db.rdb.Pipeline()
	iter := db.rdb.Scan(db.ctx, 0, db.prefixedKey(pattern), 0).Iterator()
	for iter.Next(db.ctx) {
		key := iter.Val()
		pipe.MemoryUsage(db.ctx, key)
		pipe.Del(db.ctx, key)
		result.Count++
	}
	if err := iter.Err(); err != nil {
		return result, err
	}

	cmds, err := pipe.Exec(db.ctx)
	if err != nil {
		return result, err
	}

	for i := 0; i < result.Count; i++ {
		if size, err := cmds[i*2].(*redis.IntCmd).Result(); err == nil {
			result.BytesFreed += size
		}
	}

	return result, nil
}

func (db *DB) CleanAllAudio() (CleanResult, error) {
	return db.cleanKeysByPattern("audio:*")
}

func (db *DB) CleanAllMessages() (CleanResult, error) {
	return db.cleanKeysByPattern("messages:*")
}

func (db *DB) GetAllKeys() ([]string, error) {
	var keys []string
	iter := db.rdb.Scan(db.ctx, 0, db.prefixedKey("*"), 0).Iterator()
	for iter.Next(db.ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}

func (db *DB) GetType(key string) (string, error) {
	return db.rdb.Type(db.ctx, key).Result()
}

func (db *DB) Get(key string) (string, error) {
	return db.rdb.Get(db.ctx, key).Result()
}

func (db *DB) LRange(key string, start, stop int64) ([]string, error) {
	return db.rdb.LRange(db.ctx, key, start, stop).Result()
}
