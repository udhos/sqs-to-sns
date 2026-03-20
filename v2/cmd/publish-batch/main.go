// Package main implements the tool.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"

	"github.com/udhos/sqs-to-sns/v2/internal/snsclient"
)

const maxPublishPayload = 262144

func main() {
	var topicArn string
	var roleArn string
	var endpointURL string
	var payload int
	var batch int

	flag.StringVar(&topicArn, "topic", "", "topic ARN")
	flag.StringVar(&roleArn, "role", "", "role ARN")
	flag.StringVar(&endpointURL, "endpoint", "", "endpoint URL")
	flag.IntVar(&payload, "payload", 26198, "payload size")
	flag.IntVar(&batch, "batch", 10, "batch size")
	flag.Parse()

	me := filepath.Base(os.Args[0])

	var entries []types.PublishBatchRequestEntry

	message := strings.Repeat("a", payload)

	client := snsclient.NewClient(me, topicArn, roleArn, endpointURL)

	const (
		stringType = "String"
		value      = "value1"
	)

	attr := map[string]types.MessageAttributeValue{
		"key1": {
			DataType:    aws.String(stringType),
			StringValue: aws.String(value),
		},
	}

	messageFullPayloadSize := getSNSPayloadSize(message, attr)

	slog.Info("message sizing",
		"batch", batch,
		"payload", payload,
		"attributes", messageFullPayloadSize-payload,
		"full_payload", messageFullPayloadSize,
		"batch_total", batch*messageFullPayloadSize,
		"limit", maxPublishPayload)

	for i := range batch {
		id := strconv.Itoa(i)
		entries = append(entries, types.PublishBatchRequestEntry{
			Id:                aws.String(id),
			Message:           aws.String(message),
			MessageAttributes: attr,
		})
	}

	input := &sns.PublishBatchInput{
		PublishBatchRequestEntries: entries,
		TopicArn:                   aws.String(topicArn),
	}

	result, err := client.PublishBatch(context.TODO(), input)
	if err != nil {
		slog.Error("failed to publish batch", "error", err)
		return
	}

	for _, s := range result.Successful {
		slog.Info("success",
			"Id", aws.ToString(s.Id),
			"messageId", aws.ToString(s.MessageId))
	}

	for _, f := range result.Failed {
		slog.Error("failure",
			"Id", aws.ToString(f.Id),
			"code", aws.ToString(f.Code),
			"SenderFault", f.SenderFault)
	}
}

func getSNSPayloadSize(body string, attrs map[string]types.MessageAttributeValue) int {
	size := len(body)

	for name, attr := range attrs {
		// 1. Name of the attribute
		size += len(name)

		// 2. DataType (e.g., "String", "Number", "Binary")
		if attr.DataType != nil {
			size += len(aws.ToString(attr.DataType))
		}

		// 3. String Value
		if attr.StringValue != nil {
			size += len(aws.ToString(attr.StringValue))
		}

		// 4. Binary Value
		if len(attr.BinaryValue) > 0 {
			size += len(attr.BinaryValue)
		}
	}

	return size
}
