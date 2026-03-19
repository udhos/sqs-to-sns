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

	m := newMessage(sqsMessage, now, copyAttributes, copyMessageGroupID)

	if m.snsPayloadSize != maxSnsPublishPayload {
		t.Errorf("wrong full payload size: expected=%d got=%d",
			maxSnsPublishPayload, m.snsPayloadSize)
	}

}
