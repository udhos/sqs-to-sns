package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/udhos/sqs-to-sns/v2/snsutils"
)

// SNS_TOPIC_ARN=arn:aws:sns:us-east-1:123412341234:sqs-to-sns go test -count 1 -run '^TestSNSBatchBoundary$' ./...
func TestSNSBatchBoundary(t *testing.T) {
	topicArn := os.Getenv("SNS_TOPIC_ARN")
	if topicArn == "" {
		t.Skip("Skipping integration test: SNS_TOPIC_ARN not set")
	}

	ctx := context.TODO()
	cfg, _ := awsconfig.LoadDefaultConfig(ctx)
	snsClient := sns.NewFromConfig(cfg)

	// The "Hard" Limit
	const maxSNSBatchBytes = 262144 // 256 KB

	// Test 1: Exactly 256KB (10 messages)
	// We'll make 9 messages of 20KB and 1 message that fills the remainder.
	t.Run("ExactLimit_10Msg_Success", func(t *testing.T) {
		msgs := make([]message, 10)
		totalBytes := 0

		for i := range 9 {
			m, err := createTestMessage(20000)
			if err != nil {
				t.Fatalf("Test 1 Failed: Error creating message %d: %v", i, err)
			}
			msgs[i] = m
			totalBytes += 20000
		}

		// Fill the last message to hit exactly 262,144 bytes
		remainder := maxSNSBatchBytes - totalBytes

		{
			m, err := createTestMessage(remainder)
			if err != nil {
				t.Fatalf("Test 1 Failed: Error creating remainder message: %v", err)
			}
			msgs[9] = m
		}

		pub := &publisherReal{snsClient: snsClient, awsAPITimeout: 30 * time.Second}
		q := &queue{queueCfg: queueConfig{TopicArn: topicArn}}

		sizing := GetBatchSizing(msgs)

		success, err := pub.publish(q, msgs)
		if err != nil {
			t.Errorf("Batch sizing: %s", sizing)
			t.Fatalf("Test 1 Failed: Expected success at exactly 256KB, got error: %v", err)
		}
		if len(success) != 10 {
			t.Errorf("Batch sizing: %s", sizing)
			t.Errorf("Expected 10 successful messages, got %d", len(success))
		}
		t.Logf("Batch sizing: %s", sizing)
	})

	// Test 2: 256KB + 1 Byte (Failure)
	t.Run("OverLimit_Failure", func(t *testing.T) {
		msgs := make([]message, 1)
		{
			const payloadSize = maxSNSBatchBytes + 1
			payload := strings.Repeat("a", payloadSize)

			id := getRandomID()

			sqsMessage := &sqstypes.Message{
				MessageId: aws.String(id),
				Body:      aws.String(payload),
			}

			const (
				copyAttributes     = true
				copyMessageGroupID = true
			)

			now := time.Now()

			m, _, _ := newMessageUnsafe(sqsMessage, now, copyAttributes, copyMessageGroupID)

			_, _, total := snsutils.GetSNSPayloadSize(*m.snsBatchEntry)

			if m.snsPayloadSize != payloadSize {
				t.Fatalf("Test 2 Failed: Expected message size to be %d bytes, got %d bytes", payloadSize, total)
			}

			msgs[0] = m
		}

		sizing := GetBatchSizing(msgs)

		pub := &publisherReal{snsClient: snsClient, awsAPITimeout: 30 * time.Second}
		q := &queue{queueCfg: queueConfig{TopicArn: topicArn}}

		_, err := pub.publish(q, msgs)
		if err == nil {
			t.Errorf("Batch sizing: %s", sizing)
			t.Fatal("Test 2 Failed: Expected an error for 256KB + 1 byte, but call succeeded")
		}

		t.Logf("Batch sizing: %s", sizing)
		t.Logf("Received expected error: %v", err)
	})
}
