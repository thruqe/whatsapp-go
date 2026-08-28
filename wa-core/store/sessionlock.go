// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package store

import (
	"slices"
	"sync"
)

func (device *Device) sessionLock(address string) *sync.Mutex {
	val, _ := device.sessionLocks.LoadOrStore(address, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// LockSession acquires the lock for one signal session and returns its unlock function.
func (device *Device) LockSession(address string) func() {
	lock := device.sessionLock(address)
	lock.Lock()
	return lock.Unlock
}

// LockSessions acquires session locks in sorted order to prevent deadlocks.
func (device *Device) LockSessions(addresses []string) func() {
	sorted := slices.Clone(addresses)
	slices.Sort(sorted)
	sorted = slices.Compact(sorted)
	locks := make([]*sync.Mutex, len(sorted))
	for i, address := range sorted {
		locks[i] = device.sessionLock(address)
		locks[i].Lock()
	}
	return func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Unlock()
		}
	}
}
