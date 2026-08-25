package cache

import (
	"container/list"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

type cacheEntry struct {
	key       string
	value     []byte
	expiresAt time.Time
}

func (e *cacheEntry) isExpired(now time.Time) bool {
	if e.expiresAt.IsZero() {
		return false
	}
	return now.After(e.expiresAt)
}

// MemoryStore is a thread-safe in-memory LRU cache with non-blocking TTL cleanup.
type MemoryStore struct {
	mu         sync.Mutex
	items      map[string]*list.Element
	evictList  *list.List
	maxEntries int

	stopPrune chan struct{}
	pruneDone chan struct{}
	closed    bool
}

// NewMemoryStore constructs a bounded LRU cache with an active background pruning worker.
func NewMemoryStore(maxEntries int) *MemoryStore {
	if maxEntries <= 0 {
		maxEntries = 10000
	}

	m := &MemoryStore{
		items:      make(map[string]*list.Element),
		evictList:  list.New(),
		maxEntries: maxEntries,
		stopPrune:  make(chan struct{}),
		pruneDone:  make(chan struct{}),
	}

	go m.startPruneWorker(30 * time.Second)
	return m
}

// startPruneWorker runs periodic expiration checks in chunked batches to avoid lock starvation.
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
			m.pruneExpiredBatch(now, 200)
		}
	}
}

// pruneExpiredBatch samples and purges expired keys in bounded chunks.
func (m *MemoryStore) pruneExpiredBatch(now time.Time, batchSize int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for elem := m.evictList.Back(); elem != nil && count < batchSize; {
		prev := elem.Prev()
		entry := elem.Value.(*cacheEntry)
		if entry.isExpired(now) {
			m.removeElement(elem)
			count++
		}
		elem = prev
	}
}

func (m *MemoryStore) removeElement(elem *list.Element) {
	m.evictList.Remove(elem)
	entry := elem.Value.(*cacheEntry)
	delete(m.items, entry.key)
}

// Get retrieves a string value by key.
func (m *MemoryStore) Get(ctx context.Context, key string) (string, bool, error) {
	b, ok, err := m.GetBytes(ctx, key)
	if !ok || err != nil {
		return "", ok, err
	}
	return string(b), true, nil
}

// GetBytes retrieves raw bytes by key and promotes entry to LRU front.
func (m *MemoryStore) GetBytes(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	elem, ok := m.items[key]
	if !ok {
		return nil, false, nil
	}

	entry := elem.Value.(*cacheEntry)
	if entry.isExpired(time.Now()) {
		m.removeElement(elem)
		return nil, false, nil
	}

	m.evictList.MoveToFront(elem)

	res := make([]byte, len(entry.value))
	copy(res, entry.value)
	return res, true, nil
}

// GetJSON retrieves and deserializes JSON into the target pointer.
func (m *MemoryStore) GetJSON(ctx context.Context, key string, target any) (bool, error) {
	data, ok, err := m.GetBytes(ctx, key)
	if !ok || err != nil {
		return false, err
	}
	return true, json.Unmarshal(data, target)
}

// Set stores a key-value pair and evicts the oldest entry if maxEntries capacity is hit.
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
	defer m.mu.Unlock()

	// Update existing key
	if elem, ok := m.items[key]; ok {
		m.evictList.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		entry.value = data
		entry.expiresAt = expiresAt
		return nil
	}

	// Evict oldest if full
	if m.evictList.Len() >= m.maxEntries {
		oldest := m.evictList.Back()
		if oldest != nil {
			m.removeElement(oldest)
		}
	}

	entry := &cacheEntry{
		key:       key,
		value:     data,
		expiresAt: expiresAt,
	}
	elem := m.evictList.PushFront(entry)
	m.items[key] = elem

	return nil
}

// SetJSON serializes a value to JSON and sets it.
func (m *MemoryStore) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return m.Set(ctx, key, data, ttl)
}

// Delete removes a key from cache.
func (m *MemoryStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if elem, ok := m.items[key]; ok {
		m.removeElement(elem)
	}
	return nil
}

// DeletePrefix removes all keys matching prefix.
func (m *MemoryStore) DeletePrefix(_ context.Context, prefix string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for elem := m.evictList.Front(); elem != nil; {
		next := elem.Next()
		entry := elem.Value.(*cacheEntry)
		if strings.HasPrefix(entry.key, prefix) {
			m.removeElement(elem)
		}
		elem = next
	}
	return nil
}

// Exists checks if a key exists and is unexpired.
func (m *MemoryStore) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	elem, ok := m.items[key]
	if !ok {
		return false, nil
	}

	entry := elem.Value.(*cacheEntry)
	if entry.isExpired(time.Now()) {
		m.removeElement(elem)
		return false, nil
	}

	return true, nil
}

// Clear flushes all entries.
func (m *MemoryStore) Clear(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clear(m.items)
	m.evictList.Init()
	return nil
}

// Type returns "memory".
func (m *MemoryStore) Type() string {
	return "memory"
}

// Close stops the background eviction worker and frees all allocated memory.
func (m *MemoryStore) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	close(m.stopPrune)
	clear(m.items)
	m.evictList.Init()
	m.mu.Unlock()

	<-m.pruneDone
	return nil
}
