package cache

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"
)

var (
	// ErrBufferClosed indicates that an operation was attempted on a closed buffer.
	ErrBufferClosed = errors.New("cache: write-behind buffer is closed")
)

// FlushFunc is a callback invoked with a coalesced batch of dirty entries.
type FlushFunc[K comparable, V any] func(ctx context.Context, batch map[K]V) error

// WriteBehindBuffer collects rapid mutations in memory, coalesces duplicates by key,
// and periodically flushes batches to a backing persistent store (e.g. PostgreSQL).
type WriteBehindBuffer[K comparable, V any] struct {
	mu            sync.Mutex
	dirty         map[K]V
	flushFn       FlushFunc[K, V]
	flushInterval time.Duration
	maxBatchSize  int

	flushMu  sync.Mutex // ensures only one flushFn executes at a time
	stopChan chan struct{}
	doneChan chan struct{}
	flushReq chan struct{} // channel to trigger an immediate flush
	closed   bool
}

// WriteBehindOptions configures the WriteBehindBuffer behavior.
type WriteBehindOptions struct {
	FlushInterval time.Duration
	MaxBatchSize  int
}

// NewWriteBehindBuffer creates and starts a new WriteBehindBuffer.
func NewWriteBehindBuffer[K comparable, V any](flushFn FlushFunc[K, V], opts WriteBehindOptions) *WriteBehindBuffer[K, V] {
	if opts.FlushInterval <= 0 {
		opts.FlushInterval = 1 * time.Second
	}
	if opts.MaxBatchSize <= 0 {
		opts.MaxBatchSize = 50
	}

	b := &WriteBehindBuffer[K, V]{
		dirty:         make(map[K]V),
		flushFn:       flushFn,
		flushInterval: opts.FlushInterval,
		maxBatchSize:  opts.MaxBatchSize,
		stopChan:      make(chan struct{}),
		doneChan:      make(chan struct{}),
		flushReq:      make(chan struct{}, 1),
	}

	go b.worker()
	return b
}

func (b *WriteBehindBuffer[K, V]) worker() {
	ticker := time.NewTicker(b.flushInterval)
	defer func() {
		ticker.Stop()
		close(b.doneChan)
	}()

	for {
		select {
		case <-b.stopChan:
			return
		case <-ticker.C:
			_ = b.Flush(context.Background())
		case <-b.flushReq:
			_ = b.Flush(context.Background())
		}
	}
}

// Put enqueues or updates a dirty key-value pair. Multiple updates to the same key
// within the flush window are coalesced into the latest value.
func (b *WriteBehindBuffer[K, V]) Put(key K, value V) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrBufferClosed
	}
	b.dirty[key] = value
	shouldSignal := len(b.dirty) >= b.maxBatchSize
	b.mu.Unlock()

	if shouldSignal {
		select {
		case b.flushReq <- struct{}{}:
		default:
		}
	}
	return nil
}

// PutMany enqueues multiple dirty key-value pairs at once.
func (b *WriteBehindBuffer[K, V]) PutMany(entries map[K]V) error {
	if len(entries) == 0 {
		return nil
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrBufferClosed
	}
	maps.Copy(b.dirty, entries)
	shouldSignal := len(b.dirty) >= b.maxBatchSize
	b.mu.Unlock()

	if shouldSignal {
		select {
		case b.flushReq <- struct{}{}:
		default:
		}
	}
	return nil
}

// Flush forces an immediate flush of all currently accumulated dirty entries.
func (b *WriteBehindBuffer[K, V]) Flush(ctx context.Context) error {
	b.mu.Lock()
	if len(b.dirty) == 0 {
		b.mu.Unlock()
		return nil
	}
	// Atomically swap the dirty map
	batch := b.dirty
	b.dirty = make(map[K]V, len(batch))
	b.mu.Unlock()

	b.flushMu.Lock()
	defer b.flushMu.Unlock()

	if b.flushFn != nil {
		if err := b.flushFn(ctx, batch); err != nil {
			// On error, re-insert unwritten items if they haven't been overwritten
			b.mu.Lock()
			for k, v := range batch {
				if _, exists := b.dirty[k]; !exists {
					b.dirty[k] = v
				}
			}
			b.mu.Unlock()
			return fmt.Errorf("write-behind flush failed: %w", err)
		}
	}
	return nil
}

// PendingCount returns the number of dirty items currently waiting to be flushed.
func (b *WriteBehindBuffer[K, V]) PendingCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.dirty)
}

// Close gracefully terminates the worker and drains all remaining dirty items to the backing store.
func (b *WriteBehindBuffer[K, V]) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	close(b.stopChan)
	b.mu.Unlock()

	<-b.doneChan

	// Final synchronous flush with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return b.Flush(ctx)
}
