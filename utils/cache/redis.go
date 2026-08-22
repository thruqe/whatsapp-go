package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore implements Store backed by a Redis instance or cluster.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore connects to Redis using the provided connection URL (e.g. redis://user:pass@localhost:6379/0).
func NewRedisStore(redisURL string) (*RedisStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}

	client := redis.NewClient(opts)

	// Verify connectivity with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &RedisStore{client: client}, nil
}

// Client returns the underlying go-redis Client.
func (r *RedisStore) Client() *redis.Client {
	return r.client
}

// Get retrieves a string value from Redis.
func (r *RedisStore) Get(ctx context.Context, key string) (string, bool, error) {
	val, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

// GetBytes retrieves raw bytes from Redis.
func (r *RedisStore) GetBytes(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}

// GetJSON retrieves and unmarshals JSON from Redis.
func (r *RedisStore) GetJSON(ctx context.Context, key string, target any) (bool, error) {
	data, ok, err := r.GetBytes(ctx, key)
	if !ok || err != nil {
		return false, err
	}
	return true, json.Unmarshal(data, target)
}

// Set stores a key-value pair in Redis.
func (r *RedisStore) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := serializeValue(value)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, data, ttl).Err()
}

// SetJSON serializes value to JSON and stores it in Redis.
func (r *RedisStore) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, data, ttl).Err()
}

// Delete removes a key from Redis.
func (r *RedisStore) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// DeletePrefix removes all keys matching the prefix using SCAN and pipeline DEL.
func (r *RedisStore) DeletePrefix(ctx context.Context, prefix string) error {
	pattern := prefix + "*"
	iter := r.client.Scan(ctx, 0, pattern, 0).Iterator()

	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if len(keys) >= 500 {
			if err := r.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
			keys = keys[:0]
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}

	if len(keys) > 0 {
		return r.client.Del(ctx, keys...).Err()
	}
	return nil
}

// Exists checks if a key exists in Redis.
func (r *RedisStore) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Clear flushes the current Redis database.
func (r *RedisStore) Clear(ctx context.Context) error {
	return r.client.FlushDB(ctx).Err()
}

// Type returns "redis".
func (r *RedisStore) Type() string {
	return "redis"
}

// Close closes the Redis client connection pool.
func (r *RedisStore) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}
