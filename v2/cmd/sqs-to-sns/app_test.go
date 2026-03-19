package main

import (
	"sync"
	"testing"
	"time"
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

	cfg := config{
		queues: queues,
	}

	pub := &publisherMock{}

	app := newApp(cfg,
		&receiverMock{
			latency: 10 * time.Millisecond,
			amount:  10,
		},
		pub,
	)

	go func() {
		app.run()
	}()

	time.Sleep(200 * time.Millisecond)

	app.stopReaders()

	if pub.messages != 10 {
		t.Errorf("expected %d messages, got %d", 10, pub.messages)
	}
}

type publisherMock struct {
	publishes int
	messages  int
	mu        sync.Mutex
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
		m, errMsg := createMessage(10)
		if errMsg != nil {
			return nil, stopped, errMsg
		}
		result = append(result, m)
	}

	return result, stopped, nil
}

func (r *receiverMock) stop(_ *queue) error {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
	return nil
}
