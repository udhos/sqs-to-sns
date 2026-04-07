package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/udhos/boilerplate/envconfig"
)

// go test -count 1 -run '^TestApp$' ./...
func TestApp(t *testing.T) {

	queues := []queueConfig{
		{
			QueueRoleArn: "",
			QueueURL:     "queue1",
			TopicRoleArn: "",
			TopicArn:     "topic1",
		},
	}

	queues = applyQueuesDefaults(queues)

	env := envconfig.NewSimple("test")

	t.Setenv("QUEUES", "/dev/null")

	cfg := newConfig(env)

	cfg.queues = queues

	pub := &publisherMock{}
	del := &deleterMock{}

	app := newApp(cfg,
		func(_ queueConfig) (receiver, publisher, deleter) {
			return &receiverMock{latency: 10 * time.Millisecond, amount: 10}, pub, del
		},
	)

	go func() {
		app.run()
	}()

	time.Sleep(200 * time.Millisecond)

	app.stopReaders()

	if pub.getMessages() != 10 {
		t.Errorf("publisher expected %d messages, got %d", 10, pub.messages)
	}

	if del.getMessages() != 10 {
		t.Errorf("deleter expected %d messages, got %d", 10, del.messages)
	}
}

type deleterMock struct {
	deletes  int
	messages int
	mu       sync.Mutex
}

func (d *deleterMock) getMessages() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.messages
}

func (d *deleterMock) delete(_ *queue, msg []message) ([]message, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deletes++
	d.messages += len(msg)
	return msg, nil
}

type publisherMock struct {
	publishes int
	messages  int
	mu        sync.Mutex
}

func (p *publisherMock) getMessages() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.messages
}

func (p *publisherMock) publish(_ *queue, msg []message) ([]message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.publishes++
	p.messages += len(msg)
	return msg, nil
}

type receiverMock struct {
	stopped bool
	mu      sync.Mutex
	latency time.Duration
	amount  int
}

func (r *receiverMock) receive(_ *queue) ([]message, bool, error) {
	time.Sleep(r.latency)

	var batch int

	r.mu.Lock()

	if r.amount > 10 {
		batch = 10
		r.amount -= 10
	} else {
		batch = r.amount
		r.amount = 0
	}

	stopped := r.stopped

	r.mu.Unlock()

	var result []message

	for range batch {
		m, errMsg := createTestMessage(10)
		if errMsg != nil {
			return nil, stopped, errMsg
		}
		result = append(result, m)
	}

	return result, stopped, nil
}

func (r *receiverMock) stop(_ *queue) {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
}

// ExampleGetBatchSizing demonstrates how the batch sizing string is formatted
// for a slice of SNS messages.
// go test -count 1 -run '^ExampleGetBatchSizing$' ./...
func ExampleGetBatchSizing() {
	// 1. Create a slice of messages using your helper
	// Message 1: 100 bytes
	// Message 2: 200 bytes
	msg1, _ := createTestMessage(100)
	msg2, _ := createTestMessage(200)
	msgs := []message{msg1, msg2}

	// 2. Call the function
	result := GetBatchSizing(msgs)

	// 3. Print the result for the Go test runner to verify
	fmt.Println(result)

	// Output:
	// items=2 grand_total=300: 1/2:body=100/attr=0/total_now=100/total_cached=100/attr_debug=[] 2/2:body=200/attr=0/total_now=200/total_cached=200/attr_debug=[]
}

type receiverErrMock struct {
	mu       sync.Mutex
	err      error
	mustStop bool
}

func (r *receiverErrMock) receive(_ *queue) ([]message, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return nil, r.mustStop, r.err
}

func (r *receiverErrMock) stop(_ *queue) {
	r.mu.Lock()
	r.err = context.Canceled
	r.mustStop = true
	r.mu.Unlock()
}

// go test -count 1 -run '^TestStartReaderReceiveErrorStats$' ./...
func TestStartReaderReceiveErrorStats(t *testing.T) {
	t.Run("do_not_count_shutdown_error", func(t *testing.T) {
		q := &queue{
			queueCfg: queueConfig{
				ReceiveErrorCooldown: 1 * time.Millisecond,
			},
			publishCh: make(chan message, 1),
			receive: &receiverErrMock{
				err:      context.Canceled,
				mustStop: true,
			},
			logger: slog.Default(),
		}

		initStats(&q.stats)
		q.readers.Add(1)

		app := &application{}
		app.startReader(q, false)

		if got := q.stats.receiveErrors.Load(); got != 0 {
			t.Fatalf("receiveErrors: got %d want 0", got)
		}
	})

	t.Run("count_non_shutdown_error", func(t *testing.T) {
		recv := &receiverErrMock{
			err:      errors.New("transient receive error"),
			mustStop: false,
		}

		q := &queue{
			queueCfg: queueConfig{
				ReceiveErrorCooldown: 1 * time.Millisecond,
			},
			publishCh: make(chan message, 1),
			receive:   recv,
			logger:    slog.Default(),
		}

		initStats(&q.stats)
		q.readers.Add(1)

		done := make(chan struct{})
		app := &application{}

		go func() {
			defer close(done)
			app.startReader(q, false)
		}()

		deadline := time.After(100 * time.Millisecond)
		for {
			if q.stats.receiveErrors.Load() > 0 {
				break
			}
			select {
			case <-deadline:
				t.Fatal("timeout waiting for receiveErrors increment")
			case <-time.After(1 * time.Millisecond):
			}
		}

		recv.stop(q)

		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timeout waiting startReader to stop")
		}
	})
}
