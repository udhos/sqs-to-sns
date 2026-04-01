package main

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/segmentio/ksuid"
)

// go test -count 1 -run '^TestPool$' ./...
func TestPool(t *testing.T) {
	p := newPoolV1()

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

// go test -race -run '^TestPoolConcurrency$' ./...
func TestPoolConcurrency(t *testing.T) {
	p := newPoolV1()
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

// go test -count 1 -run '^TestMessagePayload$' ./...
func TestMessagePayload(t *testing.T) {
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

func getRandomID() string {
	id, _ := ksuid.NewRandom()
	return id.String()
}

func createTestMessage(payloadSize int) (message, error) {

	id := getRandomID()

	payload := strings.Repeat("a", payloadSize)

	sqsMessage := &sqstypes.Message{
		MessageId: aws.String(id),
		Body:      aws.String(payload),
	}

	const (
		copyAttributes     = true
		copyMessageGroupID = true
	)

	now := time.Now()

	const perMessagePadding = 0

	return newMessage(sqsMessage, now, copyAttributes, copyMessageGroupID,
		perMessagePadding)
}

// go test -run '^TestPoolDeleteBehavior$' ./...
func TestPoolDeleteBehavior(t *testing.T) {
	// 1. Initialize as a Delete Pool (Limit = 0)
	p := newPoolV1()

	// 2. Create "Large" messages (e.g., 100 bytes each)
	// In a limited pool of '10', these would flush 1-by-1.
	m, _ := createTestMessage(100)

	for range 5 {
		p.add(m)
	}

	// 3. Test getFullBatch
	// It should NOT be found because we only have 5/10 messages.
	// The byte-limit (0) should be ignored.
	if _, found := p.getFullBatch(); found {
		t.Fatal("Delete pool should ignore bytes and wait for 10 items")
	}

	// 4. Test getAvailable
	// It should return all 5 messages regardless of the '0' limit.
	avail := p.getAvailable()
	if len(avail) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(avail))
	}
}
