package main

import (
	"context"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func TestForward(t *testing.T) {

	//
	// set application config environment
	//

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
	os.Setenv("EMPTY_RECEIVE_COOLDOWN", "10s")
	os.Setenv("COPY_ATTRIBUTES", "true")
	os.Setenv("DEBUG", "true")

	//
	// load data into mock sqs
	//

	const messageCount = 100

	mockSqs := &mockSqsClient{messages: map[string]*mockSqsMessage{}}
	for i := 0; i < messageCount; i++ {
		m := strconv.Itoa(i)
		mockSqs.messages[m] = &mockSqsMessage{body: m}
	}

	mockSns := &mockSnsClient{}

	newMockSqsClient := func(sessionName, queueURL, roleArn string) sqsClient {
		return mockSqs
	}

	newMockSnsClient := func(sessionName, queueURL, roleArn string) snsClient {
		return mockSns
	}

	if remain := mockSqs.count(); remain != messageCount {
		t.Errorf("wrong remaining sqs messsages: expected=%d got=%d",
			messageCount, remain)
	}

	//
	// create and run application
	//

	const me = "test-app"

	app := newApp(me, newMockSqsClient, newMockSnsClient)

	run(app)

	//
	// give application time to handle the messages
	//

	begin := time.Now()
	for {
		if time.Since(begin) > 3*time.Second {
			break
		}
		if pub := mockSns.getPublishes(); pub == messageCount {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	//
	// check that all messages have been published
	//

	if pub := mockSns.getPublishes(); pub != messageCount {
		t.Errorf("wrong sns publishes: expected=%d got=%d",
			messageCount, pub)
	}

	//
	// check that all messages have been deleted
	//

	if remain := mockSqs.count(); remain != 0 {
		t.Errorf("wrong remaining sqs messsages: expected=%d got=%d",
			0, remain)
	}
}

type mockSqsMessage struct {
	body     string
	received time.Time
}

func (m *mockSqsMessage) visible() bool {
	return time.Since(m.received) > 30*time.Second
}

type mockSqsClient struct {
	messages map[string]*mockSqsMessage
	lock     sync.Mutex
}

func (c *mockSqsClient) count() int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return len(c.messages)
}

func (c *mockSqsClient) ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	out := &sqs.ReceiveMessageOutput{}
	var count int
	for k, v := range c.messages {
		if count >= int(params.MaxNumberOfMessages) {
			break
		}
		if !v.visible() {
			continue
		}
		m := types.Message{
			Body:          aws.String(v.body),
			MessageId:     aws.String(k),
			ReceiptHandle: aws.String(k),
		}
		out.Messages = append(out.Messages, m)
		v.received = time.Now() // make message invisible for 30s
		count++
	}
	return out, nil
}

func (c *mockSqsClient) DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.messages, *params.ReceiptHandle)
	out := &sqs.DeleteMessageOutput{}
	return out, nil
}

type mockSnsClient struct {
	publishes int
	lock      sync.Mutex
}

func (c *mockSnsClient) getPublishes() int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.publishes
}

func (c *mockSnsClient) Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.publishes++
	id := "mockSnsClient.fake-publish-id"
	out := &sns.PublishOutput{
		MessageId: aws.String(id),
	}
	return out, nil
}
