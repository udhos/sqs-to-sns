package main

import (
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// go test -count 1 -run '^TestMessage$' ./...
func TestMessage(t *testing.T) {

	payload := strings.Repeat("a", maxSnsPublishPayload)

	sqsMessage := &sqstypes.Message{
		MessageId: aws.String(payload),
		Body:      aws.String(payload),
	}

	const (
		copyAttributes     = true
		copyMessageGroupID = true
	)

	now := time.Now()

	const perMessagePadding = 0

	m, err := newMessage(sqsMessage, now, copyAttributes, copyMessageGroupID,
		perMessagePadding)

	if err != nil {
		t.Errorf("message: %v", err)
	}

	if m.snsPayloadSize != maxSnsPublishPayload {
		t.Errorf("wrong full payload size: expected=%d got=%d",
			maxSnsPublishPayload, m.snsPayloadSize)
	}

}

// go test -v -count 1 -run '^TestNewMessageWithPadding$' ./...
func TestNewMessageWithPadding(t *testing.T) {
	sqsMsg := &sqstypes.Message{
		Body: aws.String("some body"),
	}

	t.Run("Accepts message within limit", func(t *testing.T) {
		// Body is small, padding is small.
		_, err := newMessage(sqsMsg, time.Now(), false, false, 100)
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("Rejects message exceeding limit due to padding", func(t *testing.T) {
		// Set padding to just under the total limit
		padding := maxSnsPublishPayload - 5
		// If the body is more than 5 bytes, this should fail.
		body := "This body is longer than five bytes"
		sqsMsg.Body = &body

		_, err := newMessage(sqsMsg, time.Now(), false, false, padding)
		if err == nil {
			t.Error("Expected error because body + padding > maxSnsPublishPayload, but got nil")
		}
	})
}
