// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package store

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/polymorfa/libsignal-protocol-go/state/record"
	"github.com/rs/zerolog"

	"go.mau.fi/util/exsync"
)

type contextKey int

const (
	contextKeySessionCache contextKey = iota
)

type sessionCacheEntry struct {
	Dirty  bool
	Found  bool
	Record *record.Session
}

type sessionCache = exsync.Map[string, sessionCacheEntry]

type sessionIterator interface {
	IterateSessions(context.Context, []string, func(string, []byte) error) error
}

type sessionLoader interface {
	IterateSession(context.Context, string, func([]byte) error) (bool, error)
}

func getSessionCache(ctx context.Context) *sessionCache {
	if ctx == nil {
		return nil
	}
	val := ctx.Value(contextKeySessionCache)
	if val == nil {
		return nil
	}
	if cache, ok := val.(*sessionCache); ok {
		return cache
	}
	return nil
}

func getCachedSession(ctx context.Context, addr string) *record.Session {
	cache := getSessionCache(ctx)
	if cache == nil {
		return nil
	}
	sess, ok := cache.Get(addr)
	if !ok {
		return nil
	}
	return sess.Record
}

func hasCachedSession(ctx context.Context, addr string) (found, cached bool) {
	cache := getSessionCache(ctx)
	if cache == nil {
		return false, false
	}
	sess, cached := cache.Get(addr)
	return sess.Found, cached
}

func putCachedSession(ctx context.Context, addr string, record *record.Session) bool {
	cache := getSessionCache(ctx)
	if cache == nil {
		return false
	}
	cache.Set(addr, sessionCacheEntry{
		Dirty:  true,
		Found:  true,
		Record: record,
	})
	return true
}

type l1SessionEntry struct {
	raw []byte
}

type l1SessionMap = exsync.Map[string, l1SessionEntry]

func (device *Device) getL1SessionCache() *l1SessionMap {
	device.l1SessionCacheLock.Lock()
	defer device.l1SessionCacheLock.Unlock()
	if device.l1SessionCache == nil {
		device.l1SessionCache = exsync.NewMap[string, l1SessionEntry]()
	}
	return device.l1SessionCache.(*l1SessionMap)
}

func (device *Device) sessionCacheKey(addr string) string {
	user := "default"
	if device.ID != nil && device.ID.User != "" {
		user = device.ID.User
	}
	return "signal:session:" + user + ":" + addr
}

func (device *Device) InvalidateL1Session(addr string) {
	if device.l1SessionCache != nil {
		if l1, ok := device.l1SessionCache.(*l1SessionMap); ok {
			l1.Delete(addr)
		}
	}
	if device.ExternalCache != nil {
		_ = device.ExternalCache.Delete(context.Background(), device.sessionCacheKey(addr))
	}
}

func (device *Device) WithCachedSessions(ctx context.Context, addresses []string) (map[string]bool, context.Context, error) {
	if len(addresses) == 0 {
		return nil, ctx, nil
	}

	wrapped := make(map[string]sessionCacheEntry, len(addresses))
	existingSessions := make(map[string]bool, len(addresses))
	for _, addr := range addresses {
		wrapped[addr] = sessionCacheEntry{Record: record.NewSession(SignalProtobufSerializer.Session, SignalProtobufSerializer.State)}
		existingSessions[addr] = false
	}
	loadSession := func(addr string, rawSess []byte) error {
		sessionRecord, err := record.NewSessionFromBytes(rawSess, SignalProtobufSerializer.Session, SignalProtobufSerializer.State)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).
				Str("address", addr).
				Msg("Failed to deserialize session")
			delete(wrapped, addr)
			delete(existingSessions, addr)
			return nil
		}
		existingSessions[addr] = true
		wrapped[addr] = sessionCacheEntry{Record: sessionRecord, Found: true}
		return nil
	}

	// 1. Check in-memory L1 cache first
	l1 := device.getL1SessionCache()
	var l1Misses []string
	for _, addr := range addresses {
		if entry, ok := l1.Get(addr); ok && len(entry.raw) > 0 {
			if err := loadSession(addr, entry.raw); err == nil && existingSessions[addr] {
				continue
			}
		}
		l1Misses = append(l1Misses, addr)
	}

	// 2. Check universal cache (Redis / In-Memory Universal Store) for L1 misses
	var missingAddresses []string
	if len(l1Misses) > 0 {
		if device.ExternalCache != nil {
			if multi, ok := device.ExternalCache.(MultiByteCache); ok {
				cacheKeys := make([]string, len(l1Misses))
				for i, a := range l1Misses {
					cacheKeys[i] = device.sessionCacheKey(a)
				}
				if fetched, err := multi.GetManyBytes(ctx, cacheKeys); err == nil && len(fetched) > 0 {
					for _, a := range l1Misses {
						key := device.sessionCacheKey(a)
						if rawBytes, found := fetched[key]; found && len(rawBytes) > 0 {
							if err := loadSession(a, rawBytes); err == nil && existingSessions[a] {
								l1.Set(a, l1SessionEntry{raw: rawBytes})
							}
						}
					}
				}
			} else {
				for _, a := range l1Misses {
					if rawBytes, ok, _ := device.ExternalCache.GetBytes(ctx, device.sessionCacheKey(a)); ok && len(rawBytes) > 0 {
						if err := loadSession(a, rawBytes); err == nil && existingSessions[a] {
							l1.Set(a, l1SessionEntry{raw: rawBytes})
						}
					}
				}
			}
		}
		for _, a := range l1Misses {
			if !existingSessions[a] {
				missingAddresses = append(missingAddresses, a)
			}
		}
	}

	// 3. Query database only for cache misses
	if len(missingAddresses) > 0 {
		if iterator, ok := device.Sessions.(sessionIterator); ok {
			if err := iterator.IterateSessions(ctx, missingAddresses, loadSession); err != nil {
				return nil, ctx, fmt.Errorf("failed to prefetch sessions: %w", err)
			}
		} else {
			sessions, err := device.Sessions.GetManySessions(ctx, missingAddresses)
			if err != nil {
				return nil, ctx, fmt.Errorf("failed to prefetch sessions: %w", err)
			}
			for addr, rawSess := range sessions {
				if rawSess != nil {
					_ = loadSession(addr, rawSess)
				}
			}
		}

		// Store retrieved sessions into L1 and Universal Cache
		if multi, ok := device.ExternalCache.(MultiByteCache); ok {
			toCache := make(map[string][]byte, len(missingAddresses))
			for _, addr := range missingAddresses {
				if sess, ok := wrapped[addr]; ok && sess.Found {
					rawBytes := sess.Record.Serialize()
					l1.Set(addr, l1SessionEntry{raw: rawBytes})
					toCache[device.sessionCacheKey(addr)] = rawBytes
				}
			}
			if len(toCache) > 0 {
				_ = multi.SetMany(ctx, toCache, 0)
			}
		} else {
			for _, addr := range missingAddresses {
				if sess, ok := wrapped[addr]; ok && sess.Found {
					rawBytes := sess.Record.Serialize()
					l1.Set(addr, l1SessionEntry{raw: rawBytes})
					if device.ExternalCache != nil {
						_ = device.ExternalCache.Set(ctx, device.sessionCacheKey(addr), rawBytes, 0)
					}
				}
			}
		}
	}

	ctx = context.WithValue(ctx, contextKeySessionCache, (*sessionCache)(exsync.NewMapWithData(wrapped)))
	return existingSessions, ctx, nil
}

func (device *Device) PutCachedSessions(ctx context.Context) error {
	cache := getSessionCache(ctx)
	if cache == nil {
		return nil
	}
	l1 := device.getL1SessionCache()
	dirtySessions := make(map[string][]byte)
	for addr, item := range cache.Iter() {
		if item.Dirty {
			raw := item.Record.Serialize()
			dirtySessions[addr] = raw
			// Update in-memory L1 session cache immediately
			l1.Set(addr, l1SessionEntry{raw: raw})
		}
	}
	if len(dirtySessions) > 0 {
		if multi, ok := device.ExternalCache.(MultiByteCache); ok {
			toCache := make(map[string][]byte, len(dirtySessions))
			for addr, raw := range dirtySessions {
				toCache[device.sessionCacheKey(addr)] = raw
			}
			_ = multi.SetMany(ctx, toCache, 0)
		} else if device.ExternalCache != nil {
			for addr, raw := range dirtySessions {
				_ = device.ExternalCache.Set(ctx, device.sessionCacheKey(addr), raw, 0)
			}
		}

		// Enqueue into coalesced session flusher instead of spawning uncoordinated goroutines
		device.getSessionCoalescer().enqueue(dirtySessions)
	}
	cache.Clear()
	return nil
}

type sessionCoalescer struct {
	mu       sync.Mutex
	dirty    map[string][]byte
	device   *Device
	flushMu  sync.Mutex
	flushReq chan struct{}
	stopChan chan struct{}
	doneChan chan struct{}
	closed   bool
}

func newSessionCoalescer(device *Device) *sessionCoalescer {
	c := &sessionCoalescer{
		dirty:    make(map[string][]byte),
		device:   device,
		flushReq: make(chan struct{}, 1),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	go c.worker()
	return c
}

func (c *sessionCoalescer) worker() {
	ticker := time.NewTicker(1 * time.Second)
	defer func() {
		ticker.Stop()
		close(c.doneChan)
	}()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			_ = c.flush(context.Background())
		case <-c.flushReq:
			_ = c.flush(context.Background())
		}
	}
}

func (c *sessionCoalescer) enqueue(entries map[string][]byte) {
	if len(entries) == 0 {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	maps.Copy(c.dirty, entries)
	shouldSignal := len(c.dirty) >= 50
	c.mu.Unlock()

	if shouldSignal {
		select {
		case c.flushReq <- struct{}{}:
		default:
		}
	}
}

func (c *sessionCoalescer) flush(ctx context.Context) error {
	c.mu.Lock()
	if len(c.dirty) == 0 {
		c.mu.Unlock()
		return nil
	}
	batch := c.dirty
	c.dirty = make(map[string][]byte, len(batch))
	c.mu.Unlock()

	c.flushMu.Lock()
	defer c.flushMu.Unlock()

	if c.device == nil || c.device.Sessions == nil {
		return nil
	}
	err := c.device.Sessions.PutManySessions(ctx, batch)
	if err != nil {
		c.mu.Lock()
		for k, v := range batch {
			if _, exists := c.dirty[k]; !exists {
				c.dirty[k] = v
			}
		}
		c.mu.Unlock()
		if c.device.Log != nil {
			c.device.Log.Warnf("Failed to persist coalesced session batch: %v", err)
		}
		return err
	}
	return nil
}

func (c *sessionCoalescer) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	close(c.stopChan)
	c.mu.Unlock()

	<-c.doneChan
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.flush(ctx)
}

func (device *Device) getSessionCoalescer() *sessionCoalescer {
	device.sessionCoalescerLock.Lock()
	defer device.sessionCoalescerLock.Unlock()
	if device.sessionCoalescer == nil {
		device.sessionCoalescer = newSessionCoalescer(device)
	}
	return device.sessionCoalescer.(*sessionCoalescer)
}

func (device *Device) flushSessions(ctx context.Context) error {
	device.sessionCoalescerLock.Lock()
	coalescer := device.sessionCoalescer
	device.sessionCoalescerLock.Unlock()
	if coalescer != nil {
		return coalescer.(*sessionCoalescer).flush(ctx)
	}
	return nil
}

func (device *Device) closeSessionCoalescer() {
	device.sessionCoalescerLock.Lock()
	coalescer := device.sessionCoalescer
	device.sessionCoalescer = nil
	device.sessionCoalescerLock.Unlock()
	if coalescer != nil {
		coalescer.(*sessionCoalescer).close()
	}
}
