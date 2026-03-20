package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

//
// deleter
//

type deleterReal struct {
	sqsClient *sqs.Client
}

func (d *deleterReal) delete(q *queue, msg []message) error {
	return fmt.Errorf("deleterReal.delete: WRITEME: %v: %d", q, len(msg))
}

//
// publisher
//

type publisherReal struct {
	snsClient *sns.Client
}

func (p *publisherReal) publish(q *queue, msg []message) ([]message, error) {
	return nil, fmt.Errorf("publisherReal.publish: WRITEME: %v: %d", q, len(msg))
}

//
// receiver
//

type receiverReal struct {
	sqsClient *sqs.Client
	stopped   bool
	mu        sync.Mutex
}

func (r *receiverReal) receive(q *queue) ([]message, bool, error) {
	const me = "receiverReal.receive"

	r.mu.Lock()
	stopped := r.stopped
	r.mu.Unlock()

	if stopped {
		return nil, stopped, nil
	}

	input := &sqs.ReceiveMessageInput{
		QueueUrl: aws.String(q.queueCfg.QueueURL),
		AttributeNames: []types.QueueAttributeName{
			"SentTimestamp",
		},
		MaxNumberOfMessages: q.queueCfg.MaxNumberOfMessages, // 1..10 (default 10)
		MessageAttributeNames: []string{
			"All",
		},
		WaitTimeSeconds: aws.ToInt32(q.queueCfg.WaitTimeSeconds), // 0..20 (default 20)
	}

	resp, errRecv := r.sqsClient.ReceiveMessage(context.TODO(), input)
	if errRecv != nil {
		return nil, stopped, errRecv
	}

	now := time.Now()

	msg := make([]message, 0, len(resp.Messages))

	for _, respMsg := range resp.Messages {

		m, errMsg := newMessage(&respMsg, now,
			aws.ToBool(q.queueCfg.CopyAttributes),
			aws.ToBool(q.queueCfg.CopyMesssageGroupID))
		if errMsg != nil {
			slog.Error(me,
				"new_message_error", errMsg,
				"queue_id", q.queueCfg.ID)
			continue
		}

		msg = append(msg, m)
	}

	return msg, stopped, nil
}

func (r *receiverReal) stop(q *queue) error {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()

	return fmt.Errorf("receiverReal.stop: WRITEME: %v", q)
}
