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
// who attempts to delete from SQS the messages that were sucessfully published.
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

const maxBatchItems = 10

func (p *pool) findBatchBelowPayloadLimit() (int, bool) {
	// 1. Handle the "No Size Limit" case (e.g., for Deletes)
	if p.snsPublishPayloadLimit <= 0 {
		// Return whichever is smaller: 10 or the current buffer size
		return min(len(p.buf), maxBatchItems), false
	}

	// 2. Handle the "Size Limited" case (e.g., for SNS Publish)

	var payloadSum int

	for i := range maxBatchItems {

		if i >= len(p.buf) {
			return len(p.buf), false // NOT restricted by payload size
		}

		m := p.buf[i]

		payloadSum += m.snsPayloadSize

		if payloadSum == p.snsPublishPayloadLimit {
			return i + 1, true // restricted by payload size
		}

		if payloadSum > p.snsPublishPayloadLimit {
			return i, true // restricted by payload size
		}
	}

	return maxBatchItems, false // NOT restricted by payload size
}

// getFullBatch extracts a full batch of 10 messages.
// it might return N<10 messages if returning
// more messages would exceed SNS publish payload byte limit.
// the 2nd return value is true if the batch was found.
func (p *pool) getFullBatch() ([]message, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	count, restrictedByPayload := p.findBatchBelowPayloadLimit()

	found := count >= maxBatchItems || (count > 0 && restrictedByPayload)

	if !found {
		return nil, false
	}

	return p.shiftUnsafe(count), true
}

// getAvailable extracts anything available up to 10 messages.
// it might return less than the available messages in
// order to not exceed SNS publish payload byte limit.
func (p *pool) getAvailable() []message {
	p.mu.Lock()
	defer p.mu.Unlock()

	count, _ := p.findBatchBelowPayloadLimit()
	if count == 0 {
		return nil
	}

	return p.shiftUnsafe(count)
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
