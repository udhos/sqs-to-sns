package main

import (
	"sync"
)

// poolV2 is suited for SNS publish in batch, since it
// accounts for SNS payload byte limit.
type poolV2 struct {
	snsPublishPayloadLimit int
	buf                    []message
	mu                     sync.Mutex
}

func newPoolV2(snsPublishPayloadLimit int) *poolV2 {

	if snsPublishPayloadLimit < 1 {
		panic("poolV2 does NOT support unlimited payload (but poolV1 does)")
	}

	return &poolV2{
		snsPublishPayloadLimit: snsPublishPayloadLimit,
		buf:                    make([]message, 0, 100),
	}
}

func (p *poolV2) add(m message) {
	p.mu.Lock()
	p.buf = append(p.buf, m)
	p.mu.Unlock()
}

func (p *poolV2) findIndices() ([]int, int) {
	var payloadSum int
	var indices []int

	// We scan the full buffer. If a message fits the current gap,
	// we take it. If it doesn't fit, it stays exactly where it is
	// to be the first candidate for the next batch.
	for i := range len(p.buf) {
		if len(indices) >= maxBatchItems {
			break
		}

		m := p.buf[i]

		if payloadSum+m.snsPayloadSize <= p.snsPublishPayloadLimit {
			payloadSum += m.snsPayloadSize
			indices = append(indices, i)
		}
	}

	return indices, payloadSum
}

func (p *poolV2) getFullBatch() ([]message, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.buf) == 0 {
		return nil, false
	}

	indices, payloadSum := p.findIndices()

	// 1. Full by Count: We hit 10 items.
	if len(indices) >= maxBatchItems {
		return p.extractUnsafe(indices), true
	}

	// 2. Full by Exact Weight: We hit the byte limit exactly.
	if payloadSum == p.snsPublishPayloadLimit {
		return p.extractUnsafe(indices), true
	}

	// 3. Full by Density: We have messages left in the buffer,
	// but we've checked EVERY one of them and NONE fit the remaining gap.
	if len(indices) < len(p.buf) {
		// Since findIndices already scanned the whole buffer and didn't
		// find anything else to fit, and we know len(indices) < len(p.buf),
		// it means every single survivor is currently too large.
		return p.extractUnsafe(indices), true
	}

	// If everyone in the buffer fits into the current batch, but we
	// aren't at 10 items or the byte limit, we wait for more SQS input.
	return nil, false
}

func (p *poolV2) getAvailable() []message {
	p.mu.Lock()
	defer p.mu.Unlock()

	indices, _ := p.findIndices()
	if len(indices) == 0 {
		return nil
	}

	return p.extractUnsafe(indices)
}

// extractUnsafe performs a non-contiguous extraction from the buffer.
func (p *poolV2) extractUnsafe(indices []int) []message {
	batch := make([]message, 0, len(indices))

	// 1. Collect messages into the batch
	for _, idx := range indices {
		batch = append(batch, p.buf[idx])
	}

	// 2. In-place filter: Shift survivors to the left in a single pass.
	// This maintains the original relative order of non-extracted messages.
	writeIdx := 0
	nextExtractedIdx := 0

	for i := 0; i < len(p.buf); i++ {
		if nextExtractedIdx < len(indices) && i == indices[nextExtractedIdx] {
			nextExtractedIdx++
			continue // Skip extracted item
		}
		p.buf[writeIdx] = p.buf[i]
		writeIdx++
	}

	// 3. Clean tail and reslice
	clear(p.buf[writeIdx:])
	p.buf = p.buf[:writeIdx]

	return batch
}
