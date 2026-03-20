package main

import (
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

func (d *deleterMock) delete(_ *queue, msg []message) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deletes++
	d.messages += len(msg)
	return nil
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
		m, errMsg := createMessage(10)
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
