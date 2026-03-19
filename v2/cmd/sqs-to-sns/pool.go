package main

import (
	"slices"
	"sync"
)

type pool struct {
	buf []message
	mu  sync.Mutex
}

func newPool() *pool {
	return &pool{
		buf: make([]message, 0, 20), // prealloc some space
	}
}

func (p *pool) isEmpty() bool {
	p.mu.Lock()
	empty := len(p.buf) == 0
	p.mu.Unlock()
	return empty
}

func (p *pool) add(m message) {
	p.mu.Lock()
	p.buf = append(p.buf, m)
	p.mu.Unlock()
}

func (p *pool) getFullBatch() ([]message, bool) {
	fatalf("return batch with less than 10 items if needed to fit total message payload in sns limit 262144")
	p.mu.Lock()
	defer p.mu.Unlock()
	found := len(p.buf) >= 10
	if !found {
		return nil, false
	}
	return p.shiftUnsafe(10), true
}

func (p *pool) getAvailable() []message {
	fatalf("return batch with less than avail items if needed to fit total message payload in sns limit 262144")
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
