package cache

import (
	"context"
	"testing"
	"time"
)

type sampleUser struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestMemoryStore_BasicOperations(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	defer store.Close()

	if store.Type() != "memory" {
		t.Errorf("expected type 'memory', got %q", store.Type())
	}

	// 1. Set and Get
	if err := store.Set(ctx, "greeting", "hello world", 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, ok, err := store.Get(ctx, "greeting")
	if err != nil || !ok || val != "hello world" {
		t.Errorf("Get failed: ok=%v val=%q err=%v", ok, val, err)
	}

	// 2. Exists
	exists, err := store.Exists(ctx, "greeting")
	if err != nil || !exists {
		t.Errorf("Exists failed: exists=%v err=%v", exists, err)
	}

	// 3. Delete
	if err := store.Delete(ctx, "greeting"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, ok, _ = store.Get(ctx, "greeting")
	if ok {
		t.Errorf("expected key to be deleted")
	}

	exists, _ = store.Exists(ctx, "greeting")
	if exists {
		t.Errorf("expected exists=false after delete")
	}
}

func TestMemoryStore_JSON(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	defer store.Close()

	u := sampleUser{Name: "Alice", Age: 30}
	if err := store.SetJSON(ctx, "user:alice", u, 0); err != nil {
		t.Fatalf("SetJSON failed: %v", err)
	}

	var fetched sampleUser
	ok, err := store.GetJSON(ctx, "user:alice", &fetched)
	if err != nil || !ok {
		t.Fatalf("GetJSON failed: ok=%v err=%v", ok, err)
	}

	if fetched.Name != "Alice" || fetched.Age != 30 {
		t.Errorf("unexpected decoded JSON: %+v", fetched)
	}
}

func TestMemoryStore_TTLExpiration(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	defer store.Close()

	if err := store.Set(ctx, "temp_key", "expiring_soon", 50*time.Millisecond); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Immediate get
	val, ok, _ := store.Get(ctx, "temp_key")
	if !ok || val != "expiring_soon" {
		t.Errorf("expected key to exist immediately")
	}

	// Wait for TTL expiration
	time.Sleep(70 * time.Millisecond)

	_, ok, _ = store.Get(ctx, "temp_key")
	if ok {
		t.Errorf("expected key to have expired")
	}

	exists, _ := store.Exists(ctx, "temp_key")
	if exists {
		t.Errorf("expected exists=false for expired key")
	}
}

func TestMemoryStore_DeletePrefixAndClear(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	defer store.Close()

	_ = store.Set(ctx, "grp:101", "data1", 0)
	_ = store.Set(ctx, "grp:102", "data2", 0)
	_ = store.Set(ctx, "usr:201", "data3", 0)

	if err := store.DeletePrefix(ctx, "grp:"); err != nil {
		t.Fatalf("DeletePrefix failed: %v", err)
	}

	if exists, _ := store.Exists(ctx, "grp:101"); exists {
		t.Errorf("expected grp:101 to be deleted by prefix")
	}
	if exists, _ := store.Exists(ctx, "grp:102"); exists {
		t.Errorf("expected grp:102 to be deleted by prefix")
	}
	if exists, _ := store.Exists(ctx, "usr:201"); !exists {
		t.Errorf("expected usr:201 to remain")
	}

	// Test Clear
	if err := store.Clear(ctx); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}
	if exists, _ := store.Exists(ctx, "usr:201"); exists {
		t.Errorf("expected usr:201 to be removed by Clear")
	}
}

func TestGlobalCache_InitFallback(t *testing.T) {
	ctx := context.Background()

	// Invalid Redis URL should gracefully fall back to MemoryStore
	s := Init("redis://invalid-host-that-does-not-exist:6379/0")
	if s == nil {
		t.Fatalf("expected non-nil store on fallback")
	}
	if s.Type() != "memory" {
		t.Errorf("expected fallback type to be 'memory', got %q", s.Type())
	}

	// Test package-level helper functions
	if err := Set(ctx, "global:key", "global_val", 0); err != nil {
		t.Fatalf("global Set failed: %v", err)
	}

	val, ok, err := Get(ctx, "global:key")
	if err != nil || !ok || val != "global_val" {
		t.Errorf("global Get failed: ok=%v val=%q err=%v", ok, val, err)
	}

	if err := Delete(ctx, "global:key"); err != nil {
		t.Fatalf("global Delete failed: %v", err)
	}
}

func TestSanitizeRedisURL(t *testing.T) {
	raw := "redis://default:supersecretpassword@redis.example.com:6379/0"
	sanitized := sanitizeRedisURL(raw)
	if sanitized == raw || sanitized == "" {
		t.Errorf("expected sanitized URL without plaintext password, got %q", sanitized)
	}
}
