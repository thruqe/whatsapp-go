package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrKeyNotFound indicates the requested key does not exist or has expired.
	ErrKeyNotFound = errors.New("cache: key not found")

	defaultStore Store
	defaultMu    sync.RWMutex
	initOnce     sync.Once
)

// Store defines the cache storage interface.
type Store interface {
	Get(ctx context.Context, key string) (string, bool, error)
	GetBytes(ctx context.Context, key string) ([]byte, bool, error)
	GetJSON(ctx context.Context, key string, target any) (bool, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
	Exists(ctx context.Context, key string) (bool, error)
	Clear(ctx context.Context) error
	Type() string
	Close() error
}

// Init initializes the global memory store with capacity constraints.
// Passing maxEntries <= 0 defaults to 10,000 entries.
func Init(maxEntries int) Store {
	defaultMu.Lock()
	defer defaultMu.Unlock()

	if defaultStore != nil {
		_ = defaultStore.Close()
	}

	if maxEntries <= 0 {
		maxEntries = 10000
	}

	defaultStore = NewMemoryStore(maxEntries)
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
		Init(10000)
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

func Get(ctx context.Context, key string) (string, bool, error) {
	return Default().Get(ctx, key)
}

func GetBytes(ctx context.Context, key string) ([]byte, bool, error) {
	return Default().GetBytes(ctx, key)
}

func GetJSON(ctx context.Context, key string, target any) (bool, error) {
	return Default().GetJSON(ctx, key, target)
}

func Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return Default().Set(ctx, key, value, ttl)
}

func SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	return Default().SetJSON(ctx, key, value, ttl)
}

func Delete(ctx context.Context, key string) error {
	return Default().Delete(ctx, key)
}

func DeletePrefix(ctx context.Context, prefix string) error {
	return Default().DeletePrefix(ctx, prefix)
}

func Exists(ctx context.Context, key string) (bool, error) {
	return Default().Exists(ctx, key)
}

func Clear(ctx context.Context) error {
	return Default().Clear(ctx)
}

func Type() string {
	return Default().Type()
}

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
