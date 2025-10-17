package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/EasterCompany/dex-discord-interface/guild"
	"github.com/bwmarrin/discordgo"
	"github.com/redis/go-redis/v9"
)

// DB is the database client
type DB struct {
	rdb *redis.Client
	ctx context.Context
}

// New returns a new database client
func New(addr string) (*DB, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	ctx := context.Background()
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("could not connect to redis: %w", err)
	}

	return &DB{rdb: rdb, ctx: ctx}, nil
}

// SaveMessage saves a message to redis
func (db *DB) SaveMessage(guildID, channelID string, m *discordgo.Message) error {
	key := fmt.Sprintf("guild:%s:channel:%s:messages", guildID, channelID)
	jsonMsg, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("could not marshal message: %w", err)
	}

	return db.rdb.RPush(db.ctx, key, jsonMsg).Err()
}

// SaveMessageHistory saves a bulk of messages to redis
func (db *DB) SaveMessageHistory(guildID, channelID string, messages []*discordgo.Message) error {
	key := fmt.Sprintf("guild:%s:channel:%s:messages", guildID, channelID)

	pipe := db.rdb.Pipeline()
	pipe.Del(db.ctx, key)
	for _, m := range messages {
		jsonMsg, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("could not marshal message: %w", err)
		}
		pipe.RPush(db.ctx, key, jsonMsg)
	}

	_, err := pipe.Exec(db.ctx)
	return err
}

// LogTranscription logs a transcription to redis
func (db *DB) LogTranscription(guildID, channelID, user, transcription string) error {
	key := fmt.Sprintf("guild:%s:channel:%s:transcriptions", guildID, channelID)
	logEntry := fmt.Sprintf("[%s] %s: %s", time.Now().Format(time.RFC3339), user, transcription)

	return db.rdb.RPush(db.ctx, key, logEntry).Err()
}

// SaveGuildState saves the state of a guild to redis
func (db *DB) SaveGuildState(guildID string, state *guild.GuildState) error {
	key := fmt.Sprintf("guild:%s:state", guildID)
	jsonState, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("could not marshal guild state: %w", err)
	}

	return db.rdb.Set(db.ctx, key, jsonState, 0).Err()
}

// LoadGuildState loads the state of a guild from redis
func (db *DB) LoadGuildState(guildID string) (*guild.GuildState, error) {
	key := fmt.Sprintf("guild:%s:state", guildID)
	jsonState, err := db.rdb.Get(db.ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("could not load guild state: %w", err)
	}

	var state guild.GuildState
	if err := json.Unmarshal([]byte(jsonState), &state); err != nil {
		return nil, fmt.Errorf("could not unmarshal guild state: %w", err)
	}

	return &state, nil
}

// GetAllGuildIDs returns all guild IDs that have state in the database
func (db *DB) GetAllGuildIDs() ([]string, error) {
	var keys []string
	iter := db.rdb.Scan(db.ctx, 0, "guild:*:state", 0).Iterator()
	for iter.Next(db.ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}
