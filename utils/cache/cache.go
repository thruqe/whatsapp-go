package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"whatsrook/logger"
)

var (
	// ErrKeyNotFound indicates the requested key does not exist or has expired.
	ErrKeyNotFound = errors.New("cache: key not found")

	defaultStore Store
	defaultMu    sync.RWMutex
	initOnce     sync.Once
)

// Store defines the universal cache storage interface.
type Store interface {
	// Get retrieves a string value by key. Returns (value, true, nil) if found.
	Get(ctx context.Context, key string) (string, bool, error)
	// GetBytes retrieves raw bytes by key. Returns (bytes, true, nil) if found.
	GetBytes(ctx context.Context, key string) ([]byte, bool, error)
	// GetJSON retrieves and deserializes JSON into target pointer.
	GetJSON(ctx context.Context, key string, target any) (bool, error)
	// Set stores a key-value pair with an optional TTL (0 for no expiration).
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	// SetJSON serializes value to JSON and stores it with an optional TTL.
	SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
	// Delete removes a key from the cache.
	Delete(ctx context.Context, key string) error
	// DeletePrefix removes all keys matching the given prefix.
	DeletePrefix(ctx context.Context, prefix string) error
	// Exists checks if a key exists and has not expired.
	Exists(ctx context.Context, key string) (bool, error)
	// Clear removes all keys from the cache.
	Clear(ctx context.Context) error
	// Type returns the cache backend type ("memory" or "redis").
	Type() string
	// Close releases resources and connections.
	Close() error
}

// Init initializes the universal cache store from a Redis URL or environment variables.
// If redisURL is empty, it checks the REDIS_URL environment variable.
// If Redis connection fails or is omitted, it falls back to the in-memory cache.
func Init(redisURL string) Store {
	defaultMu.Lock()
	defer defaultMu.Unlock()

	if defaultStore != nil {
		_ = defaultStore.Close()
	}

	if redisURL == "" {
		redisURL = os.Getenv("REDIS_URL")
	}

	if redisURL != "" && strings.TrimSpace(redisURL) != "none" {
		rStore, err := NewRedisStore(redisURL)
		if err == nil {
			Logger.Info("universal cache initialized with Redis", "url", sanitizeRedisURL(redisURL))
			defaultStore = rStore
			return defaultStore
		}
		Logger.Warn("failed to connect to Redis cache, falling back to in-memory cache", "err", err)
	}

	Logger.Info("universal cache initialized with in-memory store")
	defaultStore = NewMemoryStore()
	return defaultStore
}

// Default returns the active global Store instance, initializing it if needed.
func Default() Store {
	defaultMu.RLock()
	s := defaultStore
	defaultMu.RUnlock()

	if s != nil {
		return s
	}

	initOnce.Do(func() {
		Init("")
	})

	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultStore
}

// SetDefault overrides the active global Store instance.
func SetDefault(s Store) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultStore != nil && defaultStore != s {
		_ = defaultStore.Close()
	}
	defaultStore = s
}

// Get retrieves a string value from the default cache.
func Get(ctx context.Context, key string) (string, bool, error) {
	return Default().Get(ctx, key)
}

// GetBytes retrieves raw bytes from the default cache.
func GetBytes(ctx context.Context, key string) ([]byte, bool, error) {
	return Default().GetBytes(ctx, key)
}

// GetJSON deserializes JSON value from the default cache into target.
func GetJSON(ctx context.Context, key string, target any) (bool, error) {
	return Default().GetJSON(ctx, key, target)
}

// Set stores a key-value pair in the default cache.
func Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return Default().Set(ctx, key, value, ttl)
}

// SetJSON serializes and stores value as JSON in the default cache.
func SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	return Default().SetJSON(ctx, key, value, ttl)
}

// Delete removes a key from the default cache.
func Delete(ctx context.Context, key string) error {
	return Default().Delete(ctx, key)
}

// DeletePrefix removes all keys matching prefix from the default cache.
func DeletePrefix(ctx context.Context, prefix string) error {
	return Default().DeletePrefix(ctx, prefix)
}

// Exists checks if key exists in the default cache.
func Exists(ctx context.Context, key string) (bool, error) {
	return Default().Exists(ctx, key)
}

// Clear removes all keys from the default cache.
func Clear(ctx context.Context) error {
	return Default().Clear(ctx)
}

// Type returns the default cache backend name.
func Type() string {
	return Default().Type()
}

// Close closes the default cache store.
func Close() error {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultStore != nil {
		err := defaultStore.Close()
		defaultStore = nil
		return err
	}
	return nil
}

func serializeValue(value any) ([]byte, error) {
	switch v := value.(type) {
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	case []rune:
		return []byte(string(v)), nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		return []byte(fmt.Sprintf("%v", v)), nil
	default:
		return json.Marshal(value)
	}
}

func sanitizeRedisURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parts := strings.SplitN(rawURL, "@", 2)
	if len(parts) == 2 {
		schemeUser := parts[0]
		subParts := strings.SplitN(schemeUser, ":", 3)
		if len(subParts) == 3 {
			return fmt.Sprintf("%s:%s:****@%s", subParts[0], subParts[1], parts[1])
		}
		return fmt.Sprintf("%s:****@%s", parts[0], parts[1])
	}
	return rawURL
}
