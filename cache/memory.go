package cache

import (
	"container/list"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// cacheentry encapsulates the raw byte payload, key metadata, and absolute expiration boundary.
// this is the value container stored inside each doubly linked list element in the lru eviction chain.
type cacheEntry struct {
	key       string
	value     []byte
	expiresAt time.Time
}

// isexpired evaluates whether the current temporal marker exceeds the absolute expiration timestamp.
// a zero-value time.time indicates that no ttl was assigned, rendering the entry immortal until explicit
// or capacity-driven lru eviction.
func (e *cacheEntry) isExpired(now time.Time) bool {
	if e.expiresAt.IsZero() {
		return false
	}
	return now.After(e.expiresAt)
}

// memorystore is a thread-safe, bounded in-memory cache engine combining an $\mathcal{o}(1)$ hash map index
// with a doubly linked list for least recently used (lru) eviction and non-blocking background ttl pruning.
//
// structural invariants:
//  1. the hash map (items) maps string keys to pointers of list elements (*list.element) for constant-time lookups.
//  2. the list (evictlist) maintains temporal access ordering: the head represents the most recently accessed node,
//     while the tail represents the least recently accessed candidate for eviction.
//  3. all operations modifying or traversing items and evictlist are strictly synchronized under mu (sync.mutex).
type MemoryStore struct {
	mu         sync.Mutex
	items      map[string]*list.Element
	evictList  *list.List
	maxEntries int

	stopPrune chan struct{}
	pruneDone chan struct{}
	closed    bool
}

// newmemorystore constructs and initializes a bounded memorystore with an active background pruning worker.
// passing maxentries <= 0 defaults to 10,000 entries to guarantee bounded heap utilization.
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

	// spawn background reaper goroutine with a default sampling period of 30 seconds
	go m.startPruneWorker(30 * time.Second)
	return m
}

// startpruneworker executes a long-running periodic background sweep to harvest expired keys.
// this prevents passive memory bloat when expired keys are not actively accessed by read operations.
// todo: allow dynamic reconfiguration of the ticker interval via configuration options.
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
			// chunked eviction sweep to avoid retaining the lock across large keyspace iterations
			m.pruneExpiredBatch(now, 200)
		}
	}
}

// pruneexpiredbatch performs bounded chunked expiration starting from the tail (oldest entries).
// this limits lock hold duration to prevent write starvation on the primary request execution path.
// todo: implement an adaptive batch size algorithm based on expired-to-sampled key ratios.
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

// removeelement is an internal helper that atomizes node excision from the lru list and the lookup map.
// precondition: the caller must hold m.mu.
func (m *MemoryStore) removeElement(elem *list.Element) {
	m.evictList.Remove(elem)
	entry := elem.Value.(*cacheEntry)
	delete(m.items, entry.key)
}

// get retrieves the scalar string representation of a cached key.
// this is a high-level wrapper delegating to getbytes and casting the underlying byte slice to a string.
func (m *MemoryStore) Get(ctx context.Context, key string) (string, bool, error) {
	b, ok, err := m.GetBytes(ctx, key)
	if !ok || err != nil {
		return "", ok, err
	}
	return string(b), true, nil
}

// getbytes retrieves the raw byte payload associated with key and promotes the node to the lru head.
// this implements lazy eviction: if an accessed item is expired, it is immediately purged and returns false.
// returns a defensive deep copy of the byte slice to guarantee internal immutability against caller mutation.
// todo: evaluate providing an unsafe zero-copy variant for read-heavy performance-critical paths.
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

	// promote accessed element to head of the lru linked list
	m.evictList.MoveToFront(elem)

	// defensive copy allocation to maintain memory isolation
	res := make([]byte, len(entry.value))
	copy(res, entry.value)
	return res, true, nil
}

// getjson retrieves raw bytes for key and deserializes the payload into the target pointer reference.
// if the key is missing or expired, this returns false cleanly without mutating the target data structure.
func (m *MemoryStore) GetJSON(ctx context.Context, key string, target any) (bool, error) {
	data, ok, err := m.GetBytes(ctx, key)
	if !ok || err != nil {
		return false, err
	}
	return true, json.Unmarshal(data, target)
}

// set serializes value and persists it with an absolute expiration ttl.
//
// eviction semantics:
// 1. if the key exists, its payload and ttl are updated in place, and the node moves to the lru head.
// 2. if the key is new and capacity (maxentries) is reached, the least recently used node at the tail is evicted.
// 3. new entries are pushed directly to the front of the doubly linked list.
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

	// update existing key in place and mark as most recently used
	if elem, ok := m.items[key]; ok {
		m.evictList.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		entry.value = data
		entry.expiresAt = expiresAt
		return nil
	}

	// enforce capacity bounds by evicting the least recently used element (tail)
	if m.evictList.Len() >= m.maxEntries {
		oldest := m.evictList.Back()
		if oldest != nil {
			m.removeElement(oldest)
		}
	}

	// instantiate new entry at the lru head
	entry := &cacheEntry{
		key:       key,
		value:     data,
		expiresAt: expiresAt,
	}
	elem := m.evictList.PushFront(entry)
	m.items[key] = elem

	return nil
}

// setjson marshals arbitrary values into standard json before persisting via set.
func (m *MemoryStore) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return m.Set(ctx, key, data, ttl)
}

// delete removes an entry by key from the cache.
// this is an idempotent operation returning nil regardless of whether the key exists.
func (m *MemoryStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if elem, ok := m.items[key]; ok {
		m.removeElement(elem)
	}
	return nil
}

// deleteprefix invalidates all keys matching the lexical prefix via linear traversal $\mathcal{o}(n)$.
// note: this locks the store across the full list scan; use with discretion on large keyspaces.
// todo: consider a trie index or radix tree if prefix-based invalidation becomes a hot execution path.
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

// exists checks whether a key exists and remains unexpired without incurring copy overhead for the payload.
// if an expired key is encountered, it is immediately pruned.
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

// clear flushes all entries and re-initializes the internal hash map and doubly linked list.
func (m *MemoryStore) Clear(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clear(m.items)
	m.evictList.Init()
	return nil
}

// type returns the engine identifier string.
func (m *MemoryStore) Type() string {
	return "memory"
}

// close cleanly stops the background reaper worker, resets internal structures, and frees heap resources.
// passing force = true skips synchronization with the background pruning worker (<-m.prunedone),
// returning immediately for non-blocking teardown during emergency halts or process exit signals.
// this is idempotent: subsequent calls on an already closed instance return immediately without error.
// todo: implement a context-bounded shutdown variant with a timeout before falling back to forced termination.
func (m *MemoryStore) Close(force ...bool) error {
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

	// non-blocking fast path when caller specifies force execution
	if len(force) > 0 && force[0] {
		return nil
	}

	// block until background reaper goroutine confirms clean termination
	<-m.pruneDone
	return nil
}
