// Copyright (c) 2026 whatsrook contributors
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package store

import (
	"context"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

type memoryBufferedEvent struct {
	event     BufferedEvent
	expiresAt time.Time
}

type memoryOutgoingEvent struct {
	format    string
	plaintext []byte
	expiresAt time.Time
}

// MemoryEventBuffer implements EventBuffer entirely in memory with bounded capacity
// and TTL pruning. This eliminates high-volume write churn on persistent SQL databases
// (such as PostgreSQL) for short-lived decryption buffers and retry packets.
type MemoryEventBuffer struct {
	mu           sync.RWMutex
	decryptionMu sync.Mutex

	events         map[[32]byte]memoryBufferedEvent
	outgoingEvents map[string]memoryOutgoingEvent

	eventTTL    time.Duration
	outgoingTTL time.Duration
	maxEvents   int

	stopPrune chan struct{}
	pruneDone chan struct{}
	closed    bool
}

var _ EventBuffer = (*MemoryEventBuffer)(nil)

// MemoryEventBufferOptions configures the in-memory event buffer.
type MemoryEventBufferOptions struct {
	EventTTL    time.Duration
	OutgoingTTL time.Duration
	MaxEvents   int
}

// NewMemoryEventBuffer initializes an in-memory EventBuffer with periodic TTL reaping.
func NewMemoryEventBuffer(opts ...MemoryEventBufferOptions) *MemoryEventBuffer {
	eventTTL := 1 * time.Hour
	outgoingTTL := 30 * time.Minute
	maxEvents := 10000

	if len(opts) > 0 {
		if opts[0].EventTTL > 0 {
			eventTTL = opts[0].EventTTL
		}
		if opts[0].OutgoingTTL > 0 {
			outgoingTTL = opts[0].OutgoingTTL
		}
		if opts[0].MaxEvents > 0 {
			maxEvents = opts[0].MaxEvents
		}
	}

	buf := &MemoryEventBuffer{
		events:         make(map[[32]byte]memoryBufferedEvent),
		outgoingEvents: make(map[string]memoryOutgoingEvent),
		eventTTL:       eventTTL,
		outgoingTTL:    outgoingTTL,
		maxEvents:      maxEvents,
		stopPrune:      make(chan struct{}),
		pruneDone:      make(chan struct{}),
	}

	go buf.startReaper(1 * time.Minute)
	return buf
}

func (b *MemoryEventBuffer) startReaper(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer func() {
		ticker.Stop()
		close(b.pruneDone)
	}()

	for {
		select {
		case <-b.stopPrune:
			return
		case <-ticker.C:
			b.pruneExpired(time.Now())
		}
	}
}

func (b *MemoryEventBuffer) pruneExpired(now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for k, v := range b.events {
		if now.After(v.expiresAt) {
			delete(b.events, k)
		}
	}
	for k, v := range b.outgoingEvents {
		if now.After(v.expiresAt) {
			delete(b.outgoingEvents, k)
		}
	}
}

// GetBufferedEvent retrieves a decrypted plaintext event if still valid and unexpired.
func (b *MemoryEventBuffer) GetBufferedEvent(_ context.Context, ciphertextHash [32]byte) (*BufferedEvent, error) {
	b.mu.RLock()
	entry, ok := b.events[ciphertextHash]
	b.mu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		return nil, nil
	}

	res := entry.event
	if entry.event.Plaintext != nil {
		res.Plaintext = make([]byte, len(entry.event.Plaintext))
		copy(res.Plaintext, entry.event.Plaintext)
	}
	return &res, nil
}

// PutBufferedEvent buffers a decrypted event in memory with the configured TTL.
func (b *MemoryEventBuffer) PutBufferedEvent(_ context.Context, ciphertextHash [32]byte, plaintext []byte, serverTimestamp time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Enforce capacity bounds
	if len(b.events) >= b.maxEvents {
		now := time.Now()
		// Prune expired first
		for k, v := range b.events {
			if now.After(v.expiresAt) {
				delete(b.events, k)
			}
		}
		// If still at capacity, evict an arbitrary entry
		if len(b.events) >= b.maxEvents {
			for k := range b.events {
				delete(b.events, k)
				break
			}
		}
	}

	var ptCopy []byte
	if plaintext != nil {
		ptCopy = make([]byte, len(plaintext))
		copy(ptCopy, plaintext)
	}

	b.events[ciphertextHash] = memoryBufferedEvent{
		event: BufferedEvent{
			Plaintext:  ptCopy,
			ServerTime: serverTimestamp,
			InsertTime: time.Now(),
		},
		expiresAt: time.Now().Add(b.eventTTL),
	}
	return nil
}

// DoDecryptionTxn runs the decryption logic under an in-memory lock to prevent concurrent races.
func (b *MemoryEventBuffer) DoDecryptionTxn(ctx context.Context, fn func(context.Context) error) error {
	b.decryptionMu.Lock()
	defer b.decryptionMu.Unlock()
	return fn(ctx)
}

// ClearBufferedEventPlaintext frees plaintext bytes while retaining metadata for deduplication.
func (b *MemoryEventBuffer) ClearBufferedEventPlaintext(_ context.Context, ciphertextHash [32]byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if entry, ok := b.events[ciphertextHash]; ok {
		entry.event.Plaintext = nil
		b.events[ciphertextHash] = entry
	}
	return nil
}

// DeleteOldBufferedHashes sweeps expired events immediately.
func (b *MemoryEventBuffer) DeleteOldBufferedHashes(_ context.Context) error {
	b.pruneExpired(time.Now())
	return nil
}

func outgoingKey(chatJID types.JID, id types.MessageID) string {
	return chatJID.ToNonAD().String() + ":" + string(id)
}

// AddOutgoingEvent buffers an outgoing message payload in memory for fast retry responses.
func (b *MemoryEventBuffer) AddOutgoingEvent(_ context.Context, chatJID types.JID, id types.MessageID, format string, plaintext []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := outgoingKey(chatJID, id)
	var ptCopy []byte
	if plaintext != nil {
		ptCopy = make([]byte, len(plaintext))
		copy(ptCopy, plaintext)
	}

	b.outgoingEvents[key] = memoryOutgoingEvent{
		format:    format,
		plaintext: ptCopy,
		expiresAt: time.Now().Add(b.outgoingTTL),
	}
	return nil
}

// GetOutgoingEvent retrieves an outgoing event by chat JID and message ID.
func (b *MemoryEventBuffer) GetOutgoingEvent(_ context.Context, chatJID, altChatJID types.JID, id types.MessageID) (string, []byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	key := outgoingKey(chatJID, id)
	entry, ok := b.outgoingEvents[key]
	if !ok && !altChatJID.IsEmpty() {
		altKey := outgoingKey(altChatJID, id)
		entry, ok = b.outgoingEvents[altKey]
	}

	if !ok || time.Now().After(entry.expiresAt) {
		return "", nil, nil
	}

	ptCopy := make([]byte, len(entry.plaintext))
	copy(ptCopy, entry.plaintext)
	return entry.format, ptCopy, nil
}

// DeleteOldOutgoingEvents sweeps expired outgoing events immediately.
func (b *MemoryEventBuffer) DeleteOldOutgoingEvents(_ context.Context) error {
	b.pruneExpired(time.Now())
	return nil
}

// Close terr6minates the background reaper worker.
func (b *MemoryEventBuffer) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	close(b.stopPrune)
	b.mu.Unlock()

	<-b.pruneDone
	return nil
}
