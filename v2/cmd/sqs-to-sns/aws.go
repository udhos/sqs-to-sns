package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

//
// deleter
//

type deleterReal struct {
	sqsClient *sqs.Client
}

func (d *deleterReal) delete(q *queue, msg []message) error {
	const me = "deleterReal.delete"

	if len(msg) == 0 {
		return errors.New("deleterReal.delete: unexpected empty message list")
	}

	entries := make([]sqstypes.DeleteMessageBatchRequestEntry, len(msg))
	for i, m := range msg {
		entries[i] = sqstypes.DeleteMessageBatchRequestEntry{
			Id:            m.sqsMessage.MessageId,
			ReceiptHandle: m.sqsMessage.ReceiptHandle,
		}
	}

	input := &sqs.DeleteMessageBatchInput{
		QueueUrl: aws.String(q.queueCfg.QueueURL),
		Entries:  entries,
	}

	resp, err := d.sqsClient.DeleteMessageBatch(context.Background(), input)
	if err != nil {
		return err
	}

	// Log partial failures
	if len(resp.Failed) > 0 {
		slog.Error(me,
			"queue_id", q.queueCfg.ID,
			"failed_count", len(resp.Failed))
	}

	return nil
}

//
// publisher
//

type publisherReal struct {
	snsClient *sns.Client
}

func (p *publisherReal) publish(q *queue, msg []message) ([]message, error) {

	const me = "publishReal.publish"

	if len(msg) == 0 {
		return nil, errors.New("publisherReal.publish: unexpected empty message list")
	}

	entries := make([]snstypes.PublishBatchRequestEntry, len(msg))
	for i := range msg {
		// Entries in a batch must have an ID unique within the request.
		// We use the SQS MessageId as it's already unique and helpful for tracing.
		entry := *msg[i].snsBatchEntry
		entry.Id = msg[i].sqsMessage.MessageId
		entries[i] = entry
	}

	input := &sns.PublishBatchInput{
		TopicArn:                   aws.String(q.queueCfg.TopicArn),
		PublishBatchRequestEntries: entries,
	}

	resp, err := p.snsClient.PublishBatch(context.Background(), input)
	if err != nil {
		return nil, err
	}

	// SNS might partially fail (some messages sent, some failed).
	// We only want to return the messages that SUCCESSFULLY made it to SNS
	// so the janitor can delete them from SQS.

	// Create a map of successful IDs for fast lookup
	successIDs := make(map[string]struct{}, len(resp.Successful))
	for _, s := range resp.Successful {
		successIDs[aws.ToString(s.Id)] = struct{}{}
	}

	successMessages := make([]message, 0, len(resp.Successful))
	for _, m := range msg {
		if _, ok := successIDs[aws.ToString(m.sqsMessage.MessageId)]; ok {
			successMessages = append(successMessages, m)
		}
	}

	// Log partial failures
	if len(resp.Failed) > 0 {
		slog.Error(me,
			"queue_id", q.queueCfg.ID,
			"failed_count", len(resp.Failed))
	}

	return successMessages, nil
}

//
// receiver
//

type receiverReal struct {
	sqsClient *sqs.Client
	ctx       context.Context    // The "life" of the receiver
	cancel    context.CancelFunc // The "trigger" to kill it
	stopped   bool
	mu        sync.Mutex
}

func newReceiverReal(sqsClient *sqs.Client) *receiverReal {
	ctx, cancel := context.WithCancel(context.Background())
	return &receiverReal{
		sqsClient: sqsClient,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (r *receiverReal) receive(q *queue) ([]message, bool, error) {
	const me = "receiverReal.receive"

	r.mu.Lock()
	stopped := r.stopped
	if stopped {
		r.mu.Unlock()
		return nil, true, nil
	}
	r.mu.Unlock()

	input := &sqs.ReceiveMessageInput{
		QueueUrl: aws.String(q.queueCfg.QueueURL),
		AttributeNames: []sqstypes.QueueAttributeName{
			"SentTimestamp",
		},
		MaxNumberOfMessages: q.queueCfg.MaxNumberOfMessages, // 1..10 (default 10)
		MessageAttributeNames: []string{
			"All",
		},
		WaitTimeSeconds: aws.ToInt32(q.queueCfg.WaitTimeSeconds), // 0..20 (default 20)
	}

	resp, errRecv := r.sqsClient.ReceiveMessage(r.ctx, input)

	// Re-capture the stopped state after the block
	r.mu.Lock()
	isStopped := r.stopped
	r.mu.Unlock()

	if errRecv != nil {
		// If isStopped is true, the caller knows this error (likely context.Canceled)
		// is just the shutdown signal.
		return nil, isStopped, errRecv
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

	return msg, false, nil
}

func (r *receiverReal) stop(_ *queue) {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()

	r.cancel() // interrupt ReceiveMessage
}
