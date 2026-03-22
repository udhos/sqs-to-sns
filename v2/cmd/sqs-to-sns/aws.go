package main

import (
	"context"
	"errors"
	"fmt"
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

func (d *deleterReal) delete(q *queue, msg []message) ([]message, error) {
	const me = "deleterReal.delete"

	if len(msg) == 0 {
		return nil, errors.New("deleterReal.delete: unexpected empty message list")
	}

	entries := make([]sqstypes.DeleteMessageBatchRequestEntry, len(msg))
	for i, m := range msg {

		// Combine messageId with index to get traceability and stronger uniqueness.
		entryID := getBatchEntryID(aws.ToString(m.sqsMessage.MessageId), i)

		entries[i] = sqstypes.DeleteMessageBatchRequestEntry{
			Id:            aws.String(entryID),
			ReceiptHandle: m.sqsMessage.ReceiptHandle,
		}
	}

	input := &sqs.DeleteMessageBatchInput{
		QueueUrl: aws.String(q.queueCfg.QueueURL),
		Entries:  entries,
	}

	resp, err := d.sqsClient.DeleteMessageBatch(context.Background(), input)
	if err != nil {
		return nil, err
	}

	// Optimization: If everything succeeded, return early
	if len(resp.Failed) == 0 {
		return msg, nil
	}

	// Log partial failures.
	for _, fail := range resp.Failed {
		slog.Error(me,
			"queue_id", q.queueCfg.ID,
			"error", "partial delete failure",
			"error_code", aws.ToString(fail.Code),
			"batch_entry_id", aws.ToString(fail.Id),
			"explanation", aws.ToString(fail.Message),
			"sender_fault", fail.SenderFault,
			"failures", len(resp.Failed),
			"total_batch_size", len(msg),
		)
	}

	// We return the list of messages we successfully deleted from SQS.
	// The caller uses this information for debug logging.

	// Create a map of successful IDs for fast lookup
	successIDs := make(map[string]struct{}, len(resp.Successful))
	for _, s := range resp.Successful {
		successIDs[aws.ToString(s.Id)] = struct{}{}
	}

	successMessages := make([]message, 0, len(resp.Successful))
	for i, m := range msg {
		entryID := getBatchEntryID(aws.ToString(m.sqsMessage.MessageId), i)
		if _, ok := successIDs[entryID]; ok {
			successMessages = append(successMessages, m)
		}
	}

	return successMessages, nil
}

//
// publisher
//

type publisherReal struct {
	snsClient *sns.Client
}

func (p *publisherReal) publish(q *queue, msg []message) ([]message, error) {

	const me = "publisherReal.publish"

	if len(msg) == 0 {
		return nil, errors.New("publisherReal.publish: unexpected empty message list")
	}

	entries := make([]snstypes.PublishBatchRequestEntry, len(msg))
	for i, m := range msg {
		// Combine messageId with index to get traceability and stronger uniqueness.
		entryID := getBatchEntryID(aws.ToString(m.sqsMessage.MessageId), i)

		entry := *m.snsBatchEntry
		entry.Id = aws.String(entryID)
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

	// Optimization: If everything succeeded, return early
	if len(resp.Failed) == 0 {
		return msg, nil
	}

	// Log partial failures.
	for _, fail := range resp.Failed {
		slog.Error(me,
			"queue_id", q.queueCfg.ID,
			"error", "partial publish failure",
			"error_code", aws.ToString(fail.Code),
			"batch_entry_id", aws.ToString(fail.Id),
			"explanation", aws.ToString(fail.Message),
			"sender_fault", fail.SenderFault,
			"failures", len(resp.Failed),
			"total_batch_size", len(msg),
		)
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
	for i, m := range msg {
		entryID := getBatchEntryID(aws.ToString(m.sqsMessage.MessageId), i)
		if _, ok := successIDs[entryID]; ok {
			successMessages = append(successMessages, m)
		}
	}

	return successMessages, nil
}

func getBatchEntryID(messageID string, entryIndex int) string {
	return fmt.Sprintf("%s_%d", messageID, entryIndex)
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
