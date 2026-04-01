package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// go test -bench=. ./cmd/sqs-to-sns
// go test -bench=. -benchmem ./cmd/sqs-to-sns
// go test -bench=. -benchmem -cpu 1 ./cmd/sqs-to-sns
func BenchmarkEngine(b *testing.B) {
	numMessages := b.N

	var processedCount atomic.Int64
	var wg sync.WaitGroup
	wg.Add(numMessages)

	// 1. Setup a clean, isolated configuration
	cfg := config{
		healthAddr:           "127.0.0.1:0", // "0" tells the OS to pick any free port
		healthPath:           "/health",     // Must not be empty to avoid panic
		flushIntervalPublish: 10 * time.Millisecond,
		flushIntervalDelete:  10 * time.Millisecond,
		queues: []queueConfig{
			{
				ID:                "bench-q",
				BufferSizePublish: 5000,
				BufferSizeDelete:  5000,
				LimitReaders:      4,
				LimitPublishers:   4,
				LimitDeleters:     4,
			},
		},
	}

	// 2. High-speed Benchmark Mocks
	// These do nothing but increment counters to minimize benchmark noise.
	benchReader := &benchReceiver{total: numMessages}
	benchPub := &benchPublisher{}
	benchDel := &benchDeleter{onDone: func() {
		processedCount.Add(1)
		wg.Done()
	}}

	app := newApp(cfg, func(_ queueConfig) (receiver, publisher, deleter) {
		return benchReader, benchPub, benchDel
	})

	b.ResetTimer()
	b.ReportAllocs()

	// 3. Execution
	app.run()

	// Wait for the Janitor to signal that all messages cleared the pipe
	wg.Wait()

	b.StopTimer()
	app.stopReaders()
}

// go test -count 1 -run '^TestGracefulShutdown$' ./...
func TestGracefulShutdown(t *testing.T) {
	numMessages := 1000
	var processedCount atomic.Int64
	var wg sync.WaitGroup
	wg.Add(numMessages)

	cfg := config{
		healthAddr:           "127.0.0.1:0",
		healthPath:           "/health",
		flushIntervalPublish: 10 * time.Millisecond,
		flushIntervalDelete:  10 * time.Millisecond,
		queues: []queueConfig{{
			ID:                "test-q",
			BufferSizePublish: 1000,
			LimitPublishers:   10,
		}},
	}

	benchPub := &benchPublisher{delay: 10 * time.Millisecond}
	benchDel := &benchDeleter{onDone: func() {
		processedCount.Add(1)
		wg.Done()
	}}
	benchReader := &benchReceiver{total: numMessages}

	app := newApp(cfg, func(_ queueConfig) (receiver, publisher, deleter) {
		return benchReader, benchPub, benchDel
	})

	app.run()

	// Let the readers run for 100ms.
	// At 790ns/msg, they will easily fill the 1000-item buffer in this time.
	time.Sleep(100 * time.Millisecond)

	// Now stop the intake. The 1000 messages should already be in the pipes.
	app.stopReaders()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("Success: Processed all %d messages", processedCount.Load())
	case <-time.After(5 * time.Second):
		t.Errorf("FAIL: Only processed %d/%d. Did the workers stop too?",
			processedCount.Load(), numMessages)
	}
}

type benchReceiver struct {
	total int
	count int
	mu    sync.Mutex
}

func (r *benchReceiver) receive(_ *queue) ([]message, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	remaining := r.total - r.count
	if remaining <= 0 {
		return nil, false, nil
	}

	batchSize := min(10,
		// Cap the batch to exactly what is left
		remaining)

	r.count += batchSize

	res := make([]message, batchSize)
	for i := range batchSize {
		m, _ := createTestMessage(128)
		res[i] = m
	}
	return res, false, nil
}

func (r *benchReceiver) stop(_ *queue) {}

type benchPublisher struct {
	delay time.Duration
}

func (p *benchPublisher) publish(_ *queue, m []message) ([]message, error) {
	if p.delay > 0 {
		time.Sleep(p.delay) // Simulate AWS network lag
	}
	return m, nil
}

type benchDeleter struct {
	onDone func()
	delay  time.Duration
}

func (d *benchDeleter) delete(_ *queue, m []message) ([]message, error) {
	if d.delay > 0 {
		time.Sleep(d.delay)
	}
	for range m {
		d.onDone()
	}
	return m, nil
}
