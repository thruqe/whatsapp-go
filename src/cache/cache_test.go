package cache

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

type testPayload struct {
	ID       int      `json:"id"`
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
	IsActive bool     `json:"is_active"`
}

// testmemorystore_basiccrud verifies fundamental set, get, getbytes, and getjson operations.
func TestMemoryStore_BasicCRUD(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(100)
	defer func() { _ = store.Close() }()

	// verify string set and get
	if err := store.Set(ctx, "session:1", "active_token_xyz", time.Minute); err != nil {
		t.Fatalf("unexpected failure during set operation: %v", err)
	}

	val, hit, err := store.Get(ctx, "session:1")
	if err != nil || !hit || val != "active_token_xyz" {
		t.Fatalf("get mismatch: got (%q, %v, %v), expected (%q, true, nil)", val, hit, err, "active_token_xyz")
	}

	// verify binary payload isolation
	rawBytes := []byte{0xde, 0xad, 0xbe, 0xef}
	if err := store.Set(ctx, "binary:data", rawBytes, time.Minute); err != nil {
		t.Fatalf("failed to persist raw byte slice: %v", err)
	}

	retrievedBytes, hit, err := store.GetBytes(ctx, "binary:data")
	if err != nil || !hit || len(retrievedBytes) != 4 {
		t.Fatalf("getbytes mismatch: got len %d, hit %v, err %v", len(retrievedBytes), hit, err)
	}

	// verify json serialization and target decoding
	user := testPayload{
		ID:       101,
		Username: "thruqe",
		Roles:    []string{"admin", "maintainer"},
		IsActive: true,
	}

	if err := store.SetJSON(ctx, "user:101", user, time.Minute); err != nil {
		t.Fatalf("setjson failed: %v", err)
	}

	var decodedUser testPayload
	hit, err = store.GetJSON(ctx, "user:101", &decodedUser)
	if err != nil || !hit {
		t.Fatalf("getjson failed: hit=%v, err=%v", hit, err)
	}
	if decodedUser.ID != 101 || decodedUser.Username != "thruqe" || len(decodedUser.Roles) != 2 {
		t.Fatalf("decoded json struct fields do not match original: %+v", decodedUser)
	}
}

// testmemorystore_lrueviction verifies strict least-recently-used capacity enforcement.
func TestMemoryStore_LRUEviction(t *testing.T) {
	ctx := context.Background()
	capacity := 3
	store := NewMemoryStore(capacity)
	defer func() { _ = store.Close() }()

	// populate store to absolute capacity limit
	_ = store.Set(ctx, "k1", "v1", 0)
	_ = store.Set(ctx, "k2", "v2", 0)
	_ = store.Set(ctx, "k3", "v3", 0)

	// access k1 to promote it to the head of the lru chain
	_, hit, _ := store.Get(ctx, "k1")
	if !hit {
		t.Fatal("expected hit on k1 during lru promotion")
	}

	// insert k4, which must evict k2 (the oldest unaccessed node at the tail)
	_ = store.Set(ctx, "k4", "v4", 0)

	// k2 must be purged
	_, hit, _ = store.Get(ctx, "k2")
	if hit {
		t.Fatalf("lru violation: expected k2 to be evicted, but it was still present")
	}

	// k1, k3, and k4 must remain alive
	for _, key := range []string{"k1", "k3", "k4"} {
		if exists, _ := store.Exists(ctx, key); !exists {
			t.Fatalf("expected key %q to survive eviction cycle", key)
		}
	}
}

// testmemorystore_ttlexpiration checks lazy on-access expiration behavior.
func TestMemoryStore_TTLExpiration(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(50)
	defer func() { _ = store.Close() }()

	ttl := 20 * time.Millisecond
	if err := store.Set(ctx, "ephemeral:key", "short_lived_val", ttl); err != nil {
		t.Fatalf("failed to set key with ttl: %v", err)
	}

	// immediate lookup must succeed
	if exists, _ := store.Exists(ctx, "ephemeral:key"); !exists {
		t.Fatal("key should exist immediately following set")
	}

	// sleep past the ttl threshold to trigger lazy invalidation
	time.Sleep(30 * time.Millisecond)

	val, hit, err := store.Get(ctx, "ephemeral:key")
	if err != nil {
		t.Fatalf("unexpected error on expired lookup: %v", err)
	}
	if hit || val != "" {
		t.Fatalf("expected miss on expired key, got hit=%v, val=%q", hit, val)
	}

	// underlying map and list node must have been lazily removed
	if exists, _ := store.Exists(ctx, "ephemeral:key"); exists {
		t.Fatal("expired entry still reported as existent following failed get")
	}
}

// testmemorystore_backgroundprune verifies active background reaper key eviction.
func TestMemoryStore_BackgroundPrune(t *testing.T) {
	// construct store directly to configure an aggressive reaper frequency
	m := &MemoryStore{
		items:      make(map[string]*list.Element),
		evictList:  list.New(),
		maxEntries: 100,
		stopPrune:  make(chan struct{}),
		pruneDone:  make(chan struct{}),
	}
	go m.startPruneWorker(15 * time.Millisecond)
	defer func() { _ = m.Close() }()

	ctx := context.Background()
	// write entries with ultra-short ttl
	_ = m.Set(ctx, "reap:1", "val1", 10*time.Millisecond)
	_ = m.Set(ctx, "reap:2", "val2", 10*time.Millisecond)
	_ = m.Set(ctx, "immortal:1", "val3", 0)

	// wait for background ticker to execute at least one purge cycle
	time.Sleep(45 * time.Millisecond)

	m.mu.Lock()
	lenRemaining := len(m.items)
	m.mu.Unlock()

	if lenRemaining != 1 {
		t.Fatalf("expected background reaper to leave exactly 1 immortal key, found %d items", lenRemaining)
	}

	if exists, _ := m.Exists(ctx, "immortal:1"); !exists {
		t.Fatal("immortal key was incorrectly harvested by background worker")
	}
}

// testmemorystore_invalidation verifies delete, deleteprefix, and clear semantics.
func TestMemoryStore_Invalidation(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(100)
	defer func() { _ = store.Close() }()

	_ = store.Set(ctx, "auth:tokens:usr_1", "tok_a", 0)
	_ = store.Set(ctx, "auth:tokens:usr_2", "tok_b", 0)
	_ = store.Set(ctx, "auth:roles:usr_1", "role_admin", 0)
	_ = store.Set(ctx, "config:theme", "dark", 0)

	// delete specific key
	if err := store.Delete(ctx, "config:theme"); err != nil {
		t.Fatalf("failed to delete key: %v", err)
	}
	if exists, _ := store.Exists(ctx, "config:theme"); exists {
		t.Fatal("deleted key still reported as existing")
	}

	// delete by prefix
	if err := store.DeletePrefix(ctx, "auth:tokens:"); err != nil {
		t.Fatalf("failed during deleteprefix: %v", err)
	}

	if exists, _ := store.Exists(ctx, "auth:tokens:usr_1"); exists {
		t.Fatal("prefix deletion failed to remove auth:tokens:usr_1")
	}
	if exists, _ := store.Exists(ctx, "auth:tokens:usr_2"); exists {
		t.Fatal("prefix deletion failed to remove auth:tokens:usr_2")
	}
	if exists, _ := store.Exists(ctx, "auth:roles:usr_1"); !exists {
		t.Fatal("prefix deletion erroneously purged non-matching prefix auth:roles:usr_1")
	}

	// clear remaining namespace
	if err := store.Clear(ctx); err != nil {
		t.Fatalf("failed to clear store: %v", err)
	}
	if exists, _ := store.Exists(ctx, "auth:roles:usr_1"); exists {
		t.Fatal("key survived clear operation")
	}
}

// testmemorystore_closevariants tests graceful vs forced termination pathways.
func TestMemoryStore_CloseVariants(t *testing.T) {
	// standard graceful teardown
	store := NewMemoryStore(10)
	if err := store.Close(); err != nil {
		t.Fatalf("standard close returned unexpected error: %v", err)
	}
	// verify idempotency
	if err := store.Close(); err != nil {
		t.Fatalf("secondary close call failed: %v", err)
	}

	// forced non-blocking teardown
	storeForced := NewMemoryStore(10)
	if err := storeForced.Close(true); err != nil {
		t.Fatalf("forced close returned unexpected error: %v", err)
	}
}

// testpackagefacade exercises package-level singleton delegations and hot-swaps.
func TestPackageFacade(t *testing.T) {
	ctx := context.Background()

	// reset state before running package-level suite
	_ = Close()

	// initialize explicit store bounds
	s := Init(500)
	if s == nil || Type() != "memory" {
		t.Fatalf("expected memory driver, got %v", Type())
	}

	// test facade helpers
	if err := Set(ctx, "facade:key", "facade_val", time.Hour); err != nil {
		t.Fatalf("facade set failed: %v", err)
	}

	val, hit, err := Get(ctx, "facade:key")
	if err != nil || !hit || val != "facade_val" {
		t.Fatalf("facade get failed: got (%q, %v, %v)", val, hit, err)
	}

	if exists, _ := Exists(ctx, "facade:key"); !exists {
		t.Fatal("facade exists check returned false for existing entry")
	}

	if err := Delete(ctx, "facade:key"); err != nil {
		t.Fatalf("facade delete failed: %v", err)
	}

	// test runtime store swapping via setdefault
	mockStore := NewMemoryStore(10)
	SetDefault(mockStore)

	_ = Set(ctx, "swapped:key", "123", 0)
	if exists, _ := mockStore.Exists(ctx, "swapped:key"); !exists {
		t.Fatal("newly registered default store did not receive write operation")
	}

	_ = Close()
}

// testserialization verifies primitive fast-paths vs fallback json marshaling.
func TestSerialization(t *testing.T) {
	cases := []struct {
		input    any
		expected string
	}{
		{input: "plain_string", expected: "plain_string"},
		{input: []byte("byte_slice"), expected: "byte_slice"},
		{input: []rune{'g', 'o'}, expected: "go"},
		{input: 12345, expected: "12345"},
		{input: true, expected: "true"},
		{input: 3.14159, expected: "3.14159"},
		{input: map[string]int{"a": 1}, expected: `{"a":1}`},
	}

	for _, tc := range cases {
		b, err := serializeValue(tc.input)
		if err != nil {
			t.Fatalf("serialization failed for type %T: %v", tc.input, err)
		}
		if string(b) != tc.expected {
			t.Errorf("serializeValue(%v) = %s, expected %s", tc.input, string(b), tc.expected)
		}
	}
}

// testmemorystore_concurrency guarantees absolute safety under heavy parallel read/write contention.
func TestMemoryStore_Concurrency(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(500)
	defer func() { _ = store.Close() }()

	workers := 20
	iterations := 200
	var wg sync.WaitGroup
	wg.Add(workers * 3)

	// concurrent writers
	for i := 0; i < workers; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := fmt.Sprintf("k:%d", (workerID*iterations+j)%100)
				_ = store.Set(ctx, key, fmt.Sprintf("val-%d", j), 50*time.Millisecond)
			}
		}(i)
	}

	// concurrent readers
	for i := 0; i < workers; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := fmt.Sprintf("k:%d", (workerID*iterations+j)%100)
				_, _, _ = store.Get(ctx, key)
			}
		}(i)
	}

	// concurrent deleters
	for i := 0; i < workers; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := fmt.Sprintf("k:%d", (workerID*iterations+j)%100)
				_ = store.Delete(ctx, key)
			}
		}(i)
	}

	wg.Wait()
}
