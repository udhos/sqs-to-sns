package main

import (
	"slices"
	"sync"
)

// pool is used to accumulate messages received from sqs before publishing
// into sns.
// we aim at publishing whole 10-message batches, in order to save costs
// on AWS api calls.
// a publisher goroutine injects every message received into the pool.
// if pool.getFullBatch() extracts a 10-message batch, the publisher
// handle it immediately to the batchPublish() method (actual publishing).
// otherwise the messages sit in the pool awaiting a 10-message accumulation.
// if 10-messages are not reached within 500ms, the flusher goroutine uses
// pool.getAvailable() to extract whatever is stalled in there and flushes
// this partial batch to batchPublish().
// the whole accumulation and flushing logic is also applied to the janitor
// who attempts to delete from SQS the messages that were successfully published.
type pool interface {
	add(m message)
	getFullBatch() ([]message, bool)
	getAvailable() []message
}

// poolV1 is sufficient for deletes, since they don't need to account
// for payload byte size.
type poolV1 struct {
	buf []message
	mu  sync.Mutex
}

func newPoolV1() *poolV1 {
	return &poolV1{
		buf: make([]message, 0, 100), // prealloc some space
	}
}

func (p *poolV1) add(m message) {
	p.mu.Lock()
	p.buf = append(p.buf, m)
	p.mu.Unlock()
}

const maxBatchItems = 10

// getFullBatch extracts a full batch of 10 messages.
func (p *poolV1) getFullBatch() ([]message, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.buf) < maxBatchItems {
		return nil, false
	}

	return p.shiftUnsafe(maxBatchItems), true
}

// getAvailable extracts anything available up to 10 messages.
func (p *poolV1) getAvailable() []message {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.buf) == 0 {
		return nil
	}

	count := min(len(p.buf), maxBatchItems)

	return p.shiftUnsafe(count)
}

func (p *poolV1) shiftUnsafe(size int) []message {
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
