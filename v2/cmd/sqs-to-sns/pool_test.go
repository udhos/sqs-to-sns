package main

import (
	"sync"
	"testing"
	"time"
)

// go test -count 1 -run '^TestPool$' ./...
func TestPool(t *testing.T) {
	p := newPool()

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
			t.Fatalf("expecting 10 getAvailable: %v", m)
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
	p := newPool()
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
