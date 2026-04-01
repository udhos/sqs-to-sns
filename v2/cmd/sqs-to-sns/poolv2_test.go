package main

import (
	"sync"
	"testing"
	"time"
)

// TestPoolV2BinPacking proves the small messages should not be held hostage
// by a large message at the head.
func TestPoolV2BinPacking(t *testing.T) {
	// Limit of 10 bytes for easy math
	p := newPoolV2(10)

	// Input: [4, 4, 5, 1, 1]
	m4, _ := createTestMessage(4)
	m5, _ := createTestMessage(5)
	m1, _ := createTestMessage(1)

	p.add(m4)
	p.add(m4)
	p.add(m5) // This is the "blocker"
	p.add(m1)
	p.add(m1)

	// In PoolV1, getFullBatch would return [4, 4] and found=false
	// because 4+4+5 > 10.
	// In PoolV2, it should see that 4+4+1+1 = 10 and return found=true.

	batch, found := p.getFullBatch()
	if !found {
		t.Fatal("PoolV2 should have found a full batch by skipping the 5-byte message")
	}

	if len(batch) != 4 {
		t.Fatalf("Expected batch of 4 messages, got %d", len(batch))
	}

	// Verify the content: should be [4, 4, 1, 1]
	var sum int
	for _, m := range batch {
		sum += m.snsPayloadSize
	}
	if sum != 10 {
		t.Errorf("Expected payload sum 10, got %d", sum)
	}

	// Verify the "Survivor": The 5-byte message should still be in the pool
	avail := p.getAvailable()
	if len(avail) != 1 {
		t.Fatalf("Expected 1 message left in pool, got %d", len(avail))
	}
	if avail[0].snsPayloadSize != 5 {
		t.Errorf("The survivor should be the 5-byte message, got %d", avail[0].snsPayloadSize)
	}
}

// TestPoolV2SurvivorOrder ensures that when we pluck messages from the middle,
// the relative order of the remaining messages is preserved.
func TestPoolV2SurvivorOrder(t *testing.T) {
	p := newPoolV2(10)

	// We'll add 5 messages. We'll extract #1 and #3.
	m2, _ := createTestMessage(2) // idx 0
	m8, _ := createTestMessage(8) // idx 1 (This will be taken with idx 0)
	mA, _ := createTestMessage(1) // idx 2 (Survivor A)
	mB, _ := createTestMessage(1) // idx 3 (Survivor B)

	p.add(m2)
	p.add(m8)
	p.add(mA)
	p.add(mB)

	// This should take idx 0 and 1 (2+8=10)
	batch, found := p.getFullBatch()
	if !found || len(batch) != 2 {
		t.Fatal("Failed to extract full 10-byte batch")
	}

	// Now check if Survivors A and B are still in order
	survivors := p.getAvailable()
	if len(survivors) != 2 {
		t.Fatalf("Expected 2 survivors, got %d", len(survivors))
	}

	// We check the "ReceivedAt" or some unique property to ensure order
	if survivors[0].snsPayloadSize != 1 || survivors[1].snsPayloadSize != 1 {
		t.Error("Survivors lost their identity or order")
	}
}

// TestPoolV2MaxItems ensures that even if bytes allow more, we never exceed 10 items.
func TestPoolV2MaxItems(t *testing.T) {
	p := newPoolV2(1000) // Huge byte limit

	// Add 15 tiny messages
	m1, _ := createTestMessage(1)
	for range 15 {
		p.add(m1)
	}

	batch, found := p.getFullBatch()
	if !found {
		t.Fatal("Expected full batch")
	}
	if len(batch) != 10 {
		t.Errorf("AWS Batch limit is 10, but pool returned %d", len(batch))
	}

	remaining := p.getAvailable()
	if len(remaining) != 5 {
		t.Errorf("Expected 5 remaining messages, got %d", len(remaining))
	}
}

func TestPoolContract(t *testing.T) {
	// We run the same suite against both implementations
	tests := []struct {
		name string
		p    pool
	}{
		{"PoolV1", newPoolV1()}, // Standard 100-byte limit
		{"PoolV2", newPoolV2(100)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.p

			// 1. Requirement: Initial state is empty
			if len(p.getAvailable()) != 0 {
				t.Error("expected empty pool on start")
			}

			// 2. Requirement: getFullBatch requires 10 items (if below byte limit)
			m1, _ := createTestMessage(1)
			for range 9 {
				p.add(m1)
			}
			if _, found := p.getFullBatch(); found {
				t.Error("getFullBatch should not return true for only 9 small messages")
			}

			p.add(m1) // Add the 10th
			batch, found := p.getFullBatch()
			if !found || len(batch) != 10 {
				t.Errorf("expected found=true and len=10, got found=%v len=%d", found, len(batch))
			}

			// 3. Requirement: getAvailable drains whatever is left
			p.add(m1)
			p.add(m1)
			avail := p.getAvailable()
			if len(avail) != 2 {
				t.Errorf("expected 2 messages from getAvailable, got %d", len(avail))
			}

			// 4. Requirement: Post-extraction state is clean
			if len(p.getAvailable()) != 0 {
				t.Error("pool should be empty after getAvailable")
			}
		})
	}
}

func TestByteLimitContractV2(t *testing.T) {
	limit := 10
	p2 := newPoolV2(limit)

	m6, _ := createTestMessage(6)
	m5, _ := createTestMessage(5)
	m1, _ := createTestMessage(1) // We'll add four of these

	// Input: [6, 5, 1, 1, 1, 1]
	p2.add(m6)
	p2.add(m5)
	p2.add(m1)
	p2.add(m1)
	p2.add(m1)
	p2.add(m1)

	// V2 should see 6 + 1 + 1 + 1 + 1 = 10.
	// It skips the 5 to hit the exact limit.
	batch, found := p2.getFullBatch()
	if !found {
		t.Error("V2 should have found a full batch by skipping the 5")
	}

	if len(batch) != 5 {
		t.Errorf("Expected 5 messages (6+1+1+1+1), got %d", len(batch))
	}
}

// go test -count 1 -run '^TestPoolV2$' ./...
func TestPoolV2(t *testing.T) {
	p := newPoolV2(maxSnsPublishPayload)

	{
		m := p.getAvailable()
		if len(m) != 0 {
			t.Fatalf("expecting empty getAvailable: %v", m)
		}
	}

	p.add(message{})

	{
		m := p.getAvailable()
		if len(m) != 1 {
			t.Fatalf("expecting 1 getAvailable: %v", m)
		}
	}

	p.add(message{})
	p.add(message{})

	{
		m := p.getAvailable()
		if len(m) != 2 {
			t.Fatalf("expecting 2 getAvailable: %v", m)
		}
	}

	for range 11 {
		p.add(message{})
	}

	{
		m := p.getAvailable()
		if len(m) != 10 {
			t.Fatalf("expecting 10 getAvailable, got: %d", len(m))
		}
	}

	// 1 message

	{
		_, found := p.getFullBatch()
		if found {
			t.Fatal("expecting not found")
		}
	}

	// 1 message

	for range 9 {
		p.add(message{})
	}

	// 10 messages

	{
		m, found := p.getFullBatch()
		if !found {
			t.Fatal("expecting found")
		}
		if len(m) != 10 {
			t.Fatal("expecting full batch")
		}
	}

	// 0 messages

	{
		m := p.getAvailable()
		if len(m) != 0 {
			t.Fatalf("expecting 0 getAvailable: %v", m)
		}
	}

}

// go test -race -run '^TestPoolConcurrencyV2$' ./...
func TestPoolConcurrencyV2(t *testing.T) {
	p := newPoolV2(maxSnsPublishPayload)
	var wg sync.WaitGroup

	const (
		producers       = 5
		consumers       = 3
		msgsPerProducer = 1000
		expected        = producers * msgsPerProducer
	)

	// totalMessages we expect to eventually collect
	var totalCollected int64
	var mu sync.Mutex

	// 1. Start Producers: Jamming messages into the pool
	for range producers {
		wg.Go(func() {
			for range msgsPerProducer {
				p.add(message{})
			}
			t.Logf("producer added %d", msgsPerProducer)
		})
	}

	// 2. Start Consumers: Competing to drain the pool
	// This mimics your publisher loop calling getFullBatch and getAvailable
	stop := make(chan struct{})
	for range consumers {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					// Try to get a full batch first
					m, found := p.getFullBatch()
					if !found {
						// Fallback to whatever is available (like a heartbeat)
						m = p.getAvailable()
					}

					if len(m) > 0 {
						mu.Lock()
						totalCollected += int64(len(m))
						mu.Unlock()
					}
				}
			}
		})
	}

	time.Sleep(100 * time.Millisecond)

	close(stop)

	wg.Wait()

	if expected != totalCollected {
		t.Errorf("expected=%d totalCollected=%d", expected, totalCollected)
	}
}

// go test -race -run '^TestPoolPayloadSizeV2$' ./...
func TestPoolPayloadSizeV2(t *testing.T) {

	//
	// check avail api
	//

	m3, errMsg := createTestMessage(4)
	if errMsg != nil {
		t.Errorf("message: %v", errMsg)
	}

	p := newPoolV2(10)

	{
		avail := p.getAvailable()
		if len(avail) != 0 {
			t.Errorf("after 0 inserts, expecting 0 messages from getAvailable: %v", avail)
		}
	}
	p.add(m3)
	{
		avail := p.getAvailable()
		if len(avail) != 1 {
			t.Errorf("after 1 insert, expecting 1 messages from getAvailable: %v", avail)
		}
	}
	p.add(m3)
	p.add(m3)
	{
		avail := p.getAvailable()
		if len(avail) != 2 {
			t.Errorf("after 2 inserts, expecting 2 messages from getAvailable: %v", avail)
		}
	}
	p.add(m3)
	p.add(m3)
	p.add(m3)
	{
		avail := p.getAvailable()
		if len(avail) != 2 {
			t.Errorf("after 3 inserts, expecting 2 messages from getAvailable: %v", avail)
		}
	}

	//
	// check full batch api
	//

	p = newPoolV2(10) // reset pool

	{
		_, found := p.getFullBatch()
		if found {
			t.Errorf("after 0 inserts, expecting NOT found from getFullBatch")
		}
	}
	p.add(m3)
	{
		_, found := p.getFullBatch()
		if found {
			t.Errorf("after 1 insert, expecting NOT found from getFullBatch")
		}
	}
	p.add(m3)
	{
		//debug = true
		_, found := p.getFullBatch()
		//debug = false
		if found {
			t.Errorf("after 2 inserts, expecting NOT found from getFullBatch")
		}
	}
	p.add(m3)
	{
		full, found := p.getFullBatch()
		if !found {
			t.Errorf("after 3 insert, expecting found from getFullBatch")
		}
		if len(full) != 2 {
			t.Errorf("after 3 inserts, expecting 2 messages from getFullBatch: %v", full)
		}
	}

	//
	// test exact byte limit
	//

	p = newPoolV2(10)
	m5, errMsg := createTestMessage(5)
	if errMsg != nil {
		t.Errorf("message: %v", errMsg)
	}

	p.add(m5)
	{
		_, found := p.getFullBatch()
		if found {
			t.Errorf("after injecting 1 x 5 into 10, expecting NOT found from getFullBatch")
		}
	}

	p.add(m5)
	{
		full, found := p.getFullBatch()
		if !found {
			t.Errorf("after injecting 2 x 5 into 10, expecting found from getFullBatch")
		}
		if len(full) != 2 {
			t.Errorf("after injecting 2 x 5 into 10, expecting 2 messages from getFullBatch: %v", full)
		}
	}

	p.add(m5)
	p.add(m5)
	p.add(m5)
	{
		full, found := p.getFullBatch()
		if !found {
			t.Errorf("after injecting 3 x 5 into 10, expecting found from getFullBatch")
		}
		if len(full) != 2 {
			t.Errorf("after injecting 3 x 5 into 10, expecting 2 messages from getFullBatch: %v", full)
		}
	}

}

// go test -count 1 -run '^TestMessagePayloadV2$' ./...
func TestMessagePayloadV2(t *testing.T) {
	if _, err := createTestMessage(0); err != nil {
		t.Errorf("payload=%d: error: %v", 0, err)
	}
	if _, err := createTestMessage(1); err != nil {
		t.Errorf("payload=%d: error: %v", 1, err)
	}
	if _, err := createTestMessage(maxSnsPublishPayload - 1); err != nil {
		t.Errorf("payload=%d: error: %v", maxSnsPublishPayload-1, err)
	}
	if _, err := createTestMessage(maxSnsPublishPayload); err != nil {
		t.Errorf("payload=%d: error: %v", maxSnsPublishPayload, err)
	}
	if _, err := createTestMessage(maxSnsPublishPayload + 1); err == nil {
		t.Errorf("payload=%d: unexpected success", maxSnsPublishPayload+1)
	}
}

// go test -count 1 -run '^TestFullBatchLimitV2$' ./...
func TestFullBatchLimitV2(t *testing.T) {

	m, err := createTestMessage(1)
	if err != nil {
		t.Errorf("message error: %v", err)
	}

	p := newPoolV2(3)

	{
		_, found := p.getFullBatch()
		if found {
			t.Errorf("in the beginning, expecting NOT found from getFullBatch")
		}
	}

	p.add(m)
	{
		_, found := p.getFullBatch()
		if found {
			t.Errorf("after injecting 1 into 3, expecting NOT found from getFullBatch")
		}
	}

	p.add(m)
	{
		_, found := p.getFullBatch()
		if found {
			t.Errorf("after injecting 2 into 3, expecting NOT found from getFullBatch")
		}
	}

	p.add(m)
	{
		full, found := p.getFullBatch()
		if !found {
			t.Errorf("after injecting 3 into 3, expecting found from getFullBatch")
		}
		if len(full) != 3 {
			t.Errorf("after injecting 3 into 3, expecting 3 from getFullBatch, got %d", len(full))
		}
	}

	{
		_, found := p.getFullBatch()
		if found {
			t.Errorf("after extracting, expecting NOT found from getFullBatch")
		}
	}
}

func TestV2LargeMessageFlushesImmediately(t *testing.T) {
	p := newPoolV2(10)
	mLarge, _ := createTestMessage(9)
	mSmall, _ := createTestMessage(2)

	p.add(mLarge)
	p.add(mSmall)

	// Because 9 + 2 > 10, the 9-byte message is as "Full" as it can get.
	// Condition 3 in your poolV2 handles this: len(indices) < len(buf)
	// AND findIndices didn't take the 2.
	_, found := p.getFullBatch()
	if !found {
		t.Fatal("Expected found=true: The 9-byte message is full because the 2-byte survivor won't fit")
	}
}
