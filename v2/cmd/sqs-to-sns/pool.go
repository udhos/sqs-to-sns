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
	return &pool{}
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
	m := p.shiftUnsafe(10)
	return m, true
}

func (p *pool) getAvailable() []message {
	p.mu.Lock()
	defer p.mu.Unlock()
	size := min(len(p.buf), 10)
	return p.shiftUnsafe(size)
}

func (p *pool) shiftUnsafe(size int) []message {
	m := slices.Clone(p.buf[:size])
	p.buf = slices.Clone(p.buf[size:])
	return m
}
