package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/udhos/sqs-to-sns/sqsclient"
)

type sqsClient interface {
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

func newSqsClient(sessionName, queueURL, roleArn string) sqsClient {
	return sqsclient.NewClient(sessionName, queueURL, roleArn) // create real sqs client
}

type newSqsClientFunc func(sessionName, queueURL, roleArn string) sqsClient
