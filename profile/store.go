package profile

import (
	"context"
	"encoding/json"
	"github.com/redis/go-redis/v9"
)

type Store struct {
	Redis *redis.Client
}

func NewStore(r *redis.Client) *Store {
	return &Store{Redis: r}
}

func (s *Store) Get(ctx context.Context, userID string) (*UserProfile, error) {
	key := "user:profile:" + userID
	data, err := s.Redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // Not found, return nil (handler will generate default)
	}
	if err != nil {
		return nil, err
	}

	var p UserProfile
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) Save(ctx context.Context, p *UserProfile) error {
	key := "user:profile:" + p.UserID
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.Redis.Set(ctx, key, data, 0).Err() // No expiration
}
