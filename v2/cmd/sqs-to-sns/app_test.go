package main

import (
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

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

	app := newApp(cfg, &receiverMock{})

	go func() {
		app.run()
	}()

	time.Sleep(100 * time.Millisecond)

	t.Errorf("test message propagation")

	app.stopReaders()
}

type receiverMock struct {
	stopped bool
	mu      sync.Mutex
}

func (r *receiverMock) receive(_ *queue) ([]message, bool, error) {
	time.Sleep(2 * time.Second)

	r.mu.Lock()
	stopped := r.stopped
	r.mu.Unlock()

	body := "test"
	id := time.Now().Format(time.RFC3339)

	msg := sqstypes.Message{
		Body:      aws.String(body),
		MessageId: aws.String(id),
	}

	return []message{
		{
			sqsMessage: &msg,
			received:   time.Now(),
		},
	}, stopped, nil
}

func (r *receiverMock) stop(_ *queue) error {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
	return nil
}
