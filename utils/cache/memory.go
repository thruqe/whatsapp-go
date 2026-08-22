package cache

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

type memoryItem struct {
	value     []byte
	expiresAt time.Time
}

func (i *memoryItem) isExpired(now time.Time) bool {
	if i.expiresAt.IsZero() {
		return false
	}
	return now.After(i.expiresAt)
}

// MemoryStore is a thread-safe in-memory cache with TTL support and auto-pruning.
type MemoryStore struct {
	items     map[string]*memoryItem
	mu        sync.RWMutex
	stopPrune chan struct{}
	pruneDone chan struct{}
	closed    bool
}

// NewMemoryStore creates a new in-memory cache instance with a background cleanup worker.
func NewMemoryStore() *MemoryStore {
	m := &MemoryStore{
		items:     make(map[string]*memoryItem),
		stopPrune: make(chan struct{}),
		pruneDone: make(chan struct{}),
	}
	go m.startPruneWorker(1 * time.Minute)
	return m
}

func (m *MemoryStore) startPruneWorker(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer func() {
		ticker.Stop()
		close(m.pruneDone)
	}()

	for {
		select {
		case <-m.stopPrune:
			return
		case now := <-ticker.C:
			m.pruneExpired(now)
		}
	}
}

func (m *MemoryStore) pruneExpired(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for k, item := range m.items {
		if item.isExpired(now) {
			delete(m.items, k)
		}
	}
}

// Get retrieves a string value from memory cache.
func (m *MemoryStore) Get(_ context.Context, key string) (string, bool, error) {
	b, ok, err := m.GetBytes(context.Background(), key)
	if !ok || err != nil {
		return "", ok, err
	}
	return string(b), true, nil
}

// GetBytes retrieves raw bytes from memory cache.
func (m *MemoryStore) GetBytes(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.RLock()
	item, ok := m.items[key]
	m.mu.RUnlock()

	if !ok {
		return nil, false, nil
	}

	if item.isExpired(time.Now()) {
		m.mu.Lock()
		delete(m.items, key)
		m.mu.Unlock()
		return nil, false, nil
	}

	// Return a copy to avoid mutation
	res := make([]byte, len(item.value))
	copy(res, item.value)
	return res, true, nil
}

// GetJSON retrieves and unmarshals JSON into target.
func (m *MemoryStore) GetJSON(ctx context.Context, key string, target any) (bool, error) {
	data, ok, err := m.GetBytes(ctx, key)
	if !ok || err != nil {
		return false, err
	}
	return true, json.Unmarshal(data, target)
}

// Set stores a value in memory cache with optional TTL.
func (m *MemoryStore) Set(_ context.Context, key string, value any, ttl time.Duration) error {
	data, err := serializeValue(value)
	if err != nil {
		return err
	}

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	m.mu.Lock()
	m.items[key] = &memoryItem{
		value:     data,
		expiresAt: expiresAt,
	}
	m.mu.Unlock()
	return nil
}

// SetJSON serializes value to JSON and stores in memory cache.
func (m *MemoryStore) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return m.Set(ctx, key, data, ttl)
}

// Delete removes a key from memory cache.
func (m *MemoryStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.items, key)
	m.mu.Unlock()
	return nil
}

// DeletePrefix removes all keys matching prefix.
func (m *MemoryStore) DeletePrefix(_ context.Context, prefix string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for k := range m.items {
		if strings.HasPrefix(k, prefix) {
			delete(m.items, k)
		}
	}
	return nil
}

// Exists checks if key exists and has not expired.
func (m *MemoryStore) Exists(_ context.Context, key string) (bool, error) {
	m.mu.RLock()
	item, ok := m.items[key]
	m.mu.RUnlock()

	if !ok {
		return false, nil
	}

	if item.isExpired(time.Now()) {
		m.mu.Lock()
		delete(m.items, key)
		m.mu.Unlock()
		return false, nil
	}

	return true, nil
}

// Clear flushes all keys from memory cache.
func (m *MemoryStore) Clear(_ context.Context) error {
	m.mu.Lock()
	clear(m.items)
	m.mu.Unlock()
	return nil
}

// Type returns "memory".
func (m *MemoryStore) Type() string {
	return "memory"
}

// Close stops the background eviction worker and frees memory.
func (m *MemoryStore) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	close(m.stopPrune)
	clear(m.items)
	m.mu.Unlock()

	<-m.pruneDone
	return nil
}
