// Copyright (c) 2025 Bob Vawter (bob@vawter.org)
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
//
// SPDX-License-Identifier: MIT

package message

import (
	"runtime"
	"sync"
	"unique"
	"weak"
)

// cmap is a canonicalizing map.
type cmap[K comparable, V any] struct {
	new func(unique.Handle[K]) *V
	mu  struct {
		sync.RWMutex
		data map[unique.Handle[K]]weak.Pointer[V]
	}
}

type entry[K comparable, V any] struct {
	h unique.Handle[K]
	v weak.Pointer[V]
}

// cleanup conditionally deletes map entries.
func (m *cmap[K, V]) cleanup(task *entry[K, V]) {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := m.mu.data[task.h]
	if found == task.v {
		delete(m.mu.data, task.h)
	}
}

func (m *cmap[K, V]) get(key K) *V {
	h := unique.Make(key)

	// Fast-path.
	found, ok := m.peek(h)
	if ok {
		return found
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check.
	if ptr, ok := m.mu.data[h]; ok {
		if value := ptr.Value(); value != nil {
			return value
		}
	}

	// Assign with conditional-delete cleanup.
	ret := m.new(h)
	ptr := weak.Make(ret)
	if m.mu.data == nil {
		m.mu.data = make(map[unique.Handle[K]]weak.Pointer[V])
	}
	m.mu.data[h] = ptr
	runtime.AddCleanup(ret, m.cleanup, &entry[K, V]{h, ptr})
	return ret
}

func (m *cmap[K, V]) peek(h unique.Handle[K]) (*V, bool) {
	m.mu.RLock()
	ptr, ok := m.mu.data[h]
	m.mu.RUnlock()

	if !ok {
		return nil, false
	}

	found := ptr.Value()
	if found == nil {
		return nil, false
	}
	return found, true
}
