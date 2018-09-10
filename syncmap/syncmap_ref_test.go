// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syncmap_test

import (
	"sync"
	"sync/atomic"
)

// This file contains reference map implementations for unit-tests.

// mapInterface is the interface Map implements.
type mapInterface interface {
	Load(KeyT) (ValueT, bool)
	Store(key KeyT, value ValueT)
	LoadOrStore(key KeyT, value ValueT) (actual ValueT, loaded bool)
	Delete(KeyT)
	Range(func(key KeyT, value ValueT) (shouldContinue bool))
}

// RWMutexMap is an implementation of mapInterface using a sync.RWMutex.
type RWMutexMap struct {
	mu    sync.RWMutex
	dirty map[KeyT]ValueT
}

func (m *RWMutexMap) Load(key KeyT) (value ValueT, ok bool) {
	m.mu.RLock()
	value, ok = m.dirty[key]
	m.mu.RUnlock()
	return
}

func (m *RWMutexMap) Store(key KeyT, value ValueT) {
	m.mu.Lock()
	if m.dirty == nil {
		m.dirty = make(map[KeyT]ValueT)
	}
	m.dirty[key] = value
	m.mu.Unlock()
}

func (m *RWMutexMap) LoadOrStore(key KeyT, value ValueT) (actual ValueT, loaded bool) {
	m.mu.Lock()
	actual, loaded = m.dirty[key]
	if !loaded {
		actual = value
		if m.dirty == nil {
			m.dirty = make(map[KeyT]ValueT)
		}
		m.dirty[key] = value
	}
	m.mu.Unlock()
	return actual, loaded
}

func (m *RWMutexMap) Delete(key KeyT) {
	m.mu.Lock()
	delete(m.dirty, key)
	m.mu.Unlock()
}

func (m *RWMutexMap) Range(f func(key KeyT, value ValueT) (shouldContinue bool)) {
	m.mu.RLock()
	keys := make([]KeyT, 0, len(m.dirty))
	for k := range m.dirty {
		keys = append(keys, k)
	}
	m.mu.RUnlock()

	for _, k := range keys {
		v, ok := m.Load(k)
		if !ok {
			continue
		}
		if !f(k, v) {
			break
		}
	}
}

// DeepCopyMap is an implementation of mapInterface using a Mutex and
// atomic.Value.  It makes deep copies of the map on every write to avoid
// acquiring the Mutex in Load.
type DeepCopyMap struct {
	mu    sync.Mutex
	clean atomic.Value
}

func (m *DeepCopyMap) Load(key KeyT) (value ValueT, ok bool) {
	clean, _ := m.clean.Load().(map[KeyT]ValueT)
	value, ok = clean[key]
	return value, ok
}

func (m *DeepCopyMap) Store(key KeyT, value ValueT) {
	m.mu.Lock()
	dirty := m.dirty()
	dirty[key] = value
	m.clean.Store(dirty)
	m.mu.Unlock()
}

func (m *DeepCopyMap) LoadOrStore(key KeyT, value ValueT) (actual ValueT, loaded bool) {
	clean, _ := m.clean.Load().(map[KeyT]ValueT)
	actual, loaded = clean[key]
	if loaded {
		return actual, loaded
	}

	m.mu.Lock()
	// Reload clean in case it changed while we were waiting on m.mu.
	clean, _ = m.clean.Load().(map[KeyT]ValueT)
	actual, loaded = clean[key]
	if !loaded {
		dirty := m.dirty()
		dirty[key] = value
		actual = value
		m.clean.Store(dirty)
	}
	m.mu.Unlock()
	return actual, loaded
}

func (m *DeepCopyMap) Delete(key KeyT) {
	m.mu.Lock()
	dirty := m.dirty()
	delete(dirty, key)
	m.clean.Store(dirty)
	m.mu.Unlock()
}

func (m *DeepCopyMap) Range(f func(key KeyT, value ValueT) (shouldContinue bool)) {
	clean, _ := m.clean.Load().(map[KeyT]ValueT)
	for k, v := range clean {
		if !f(k, v) {
			break
		}
	}
}

func (m *DeepCopyMap) dirty() map[KeyT]ValueT {
	clean, _ := m.clean.Load().(map[KeyT]ValueT)
	dirty := make(map[KeyT]ValueT, len(clean)+1)
	for k, v := range clean {
		dirty[k] = v
	}
	return dirty
}
