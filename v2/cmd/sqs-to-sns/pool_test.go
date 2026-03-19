package main

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// go test -count 1 -run '^TestPool$' ./...
func TestPool(t *testing.T) {
	p := newPool(maxSnsPublishPayload)

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
	p := newPool(maxSnsPublishPayload)
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

// go test -race -run '^TestPoolPayloadSize$' ./...
func TestPoolPayloadSize(t *testing.T) {

	//
	// check avail api
	//

	m3, errMsg := createMessage(4)
	if errMsg != nil {
		t.Errorf("message: %v", errMsg)
	}

	p := newPool(10)

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

	p = newPool(10) // reset pool

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

	p = newPool(10)
	m5, errMsg := createMessage(5)
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

// go test -count 1 -run '^TestMessagePayload$' ./...
func TestMessagePayload(t *testing.T) {
	if _, err := createMessage(0); err != nil {
		t.Errorf("payload=%d: error: %v", 0, err)
	}
	if _, err := createMessage(1); err != nil {
		t.Errorf("payload=%d: error: %v", 1, err)
	}
	if _, err := createMessage(maxSnsPublishPayload - 1); err != nil {
		t.Errorf("payload=%d: error: %v", maxSnsPublishPayload-1, err)
	}
	if _, err := createMessage(maxSnsPublishPayload); err != nil {
		t.Errorf("payload=%d: error: %v", maxSnsPublishPayload, err)
	}
	if _, err := createMessage(maxSnsPublishPayload + 1); err == nil {
		t.Errorf("payload=%d: unexpected success", maxSnsPublishPayload+1)
	}
}

func createMessage(payloadSize int) (message, error) {

	payload := strings.Repeat("a", payloadSize)

	sqsMessage := &sqstypes.Message{
		MessageId: aws.String(payload),
		Body:      aws.String(payload),
	}

	const (
		copyAttributes     = true
		copyMessageGroupID = true
	)

	now := time.Now()

	return newMessage(sqsMessage, now, copyAttributes, copyMessageGroupID)
}
