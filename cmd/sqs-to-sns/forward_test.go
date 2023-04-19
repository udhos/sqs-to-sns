package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

func TestQueue(t *testing.T) {

	os.Setenv("QUEUES", "testdata/test-queues.yaml")

	os.Setenv("HEALTH_ADDR", ":8888")
	os.Setenv("HEALTH_PATH", "/health")

	os.Setenv("METRICS_ADDR", ":3000")
	os.Setenv("METRICS_PATH", "/metrics")
	os.Setenv("METRICS_NAMESPACE", "sqstosns")

	// default values
	os.Setenv("QUEUE_ROLE_ARN", "")
	os.Setenv("TOPIC_ROLE_ARN", "")
	os.Setenv("READERS", "1")
	os.Setenv("WRITERS", "1")
	os.Setenv("BUFFER", "10")
	os.Setenv("READ_ERROR_COOLDOWN", "10s")
	os.Setenv("WRITE_ERROR_COOLDOWN", "10s")
	os.Setenv("DELETE_ERROR_COOLDOWN", "10s")
	os.Setenv("COPY_ATTRIBUTES", "true")
	os.Setenv("DEBUG", "true")

	const me = "test-app"

	app := newApp(me, newMockSqsClient, newMockSnsClient)

	run(app)

	time.Sleep(1 * time.Second)

	t.Logf("done")
}

func newMockSqsClient(sessionName, queueURL, roleArn string) sqsClient {
	return &mockSqsClient{}
}

type mockSqsClient struct {
}

func (c *mockSqsClient) ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	return nil, errors.New("recv")
}

func (c *mockSqsClient) DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	return nil, errors.New("del")
}

func newMockSnsClient(sessionName, queueURL, roleArn string) snsClient {
	return &mockSnsClient{}
}

type mockSnsClient struct {
}

func (c *mockSnsClient) Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error) {
	return nil, errors.New("pub")
}
