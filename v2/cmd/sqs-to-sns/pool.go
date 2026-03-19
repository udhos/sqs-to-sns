package main

import (
	"slices"
	"sync"
)

type pool struct {
	snsPublishPayloadLimit int
	buf                    []message
	mu                     sync.Mutex
}

func newPool(snsPublishPayloadLimit int) *pool {
	return &pool{
		snsPublishPayloadLimit: snsPublishPayloadLimit,
		buf:                    make([]message, 0, 100), // prealloc some space
	}
}

func (p *pool) add(m message) {
	p.mu.Lock()
	p.buf = append(p.buf, m)
	p.mu.Unlock()
}

func (p *pool) getFullBatch() ([]message, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	found := len(p.buf) >= 10
	if !found {
		return nil, false
	}
	return p.shiftUnsafe(10), true
}

func (p *pool) getAvailable() []message {
	p.mu.Lock()
	defer p.mu.Unlock()
	size := min(len(p.buf), 10)
	if size == 0 {
		return nil
	}
	return p.shiftUnsafe(size)
}

func (p *pool) shiftUnsafe(size int) []message {
	// 1. Create the batch to return.
	// We still clone the batch itself so the caller has their own data.
	m := slices.Clone(p.buf[:size])

	// 2. Shift the remaining messages to the start of the array.
	// This is a high-speed memory move.
	n := copy(p.buf, p.buf[size:])

	// 3. CRITICAL: Zero out the "dead" space.
	// Since the message struct has a pointer, this allows the GC to
	// reclaim the memory the pointer was hitting.
	clear(p.buf[n:])

	// 4. Reslice to the new length while keeping the capacity.
	p.buf = p.buf[:n]

	return m
}
