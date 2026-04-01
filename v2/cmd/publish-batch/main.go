// Package main implements the tool.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/udhos/sqs-to-sns/v2/internal/snsclient"
	"github.com/udhos/sqs-to-sns/v2/snsutils"
)

const maxPublishPayload = 262144

func main() {
	var topicArn string
	var roleArn string
	var endpointURL string
	var payload int
	var batch int
	var attributes bool

	flag.StringVar(&topicArn, "topic", "", "topic ARN")
	flag.StringVar(&roleArn, "role", "", "role ARN")
	flag.StringVar(&endpointURL, "endpoint", "", "endpoint URL")
	flag.IntVar(&payload, "payload", 26198, fmt.Sprintf("payload size (max %d)", maxPublishPayload))
	flag.IntVar(&batch, "batch", 10, "batch size")
	flag.BoolVar(&attributes, "attributes", false, "include message attributes")
	flag.Parse()

	me := filepath.Base(os.Args[0])

	var entries []snstypes.PublishBatchRequestEntry

	message := strings.Repeat("a", payload)

	client := snsclient.NewClient(me, topicArn, roleArn, endpointURL)

	const (
		stringType = "String"
		value      = "value1"
	)

	entry := snstypes.PublishBatchRequestEntry{
		Message: aws.String(message),
	}

	if attributes {
		attr := map[string]snstypes.MessageAttributeValue{
			"key1": {
				DataType:    aws.String(stringType),
				StringValue: aws.String(value),
			},
		}
		entry.MessageAttributes = attr
	}

	messageBodyPayloadSize, messageAttrPayloadSize, messageFullPayloadSize := snsutils.GetSNSPayloadSize(entry)

	slog.Info("message sizing",
		"batch", batch,
		"payload", payload,
		"attributes", messageAttrPayloadSize,
		"full_payload", messageFullPayloadSize,
		"batch_total", batch*messageFullPayloadSize,
		"limit", maxPublishPayload)

	for i := range batch {
		entry.Id = aws.String(strconv.Itoa(i))
		entries = append(entries, entry)
	}

	input := &sns.PublishBatchInput{
		PublishBatchRequestEntries: entries,
		TopicArn:                   aws.String(topicArn),
	}

	result, err := client.PublishBatch(context.TODO(), input)
	if err != nil {
		slog.Error("failed to publish batch",
			"bodySize", messageBodyPayloadSize,
			"attributesSize", messageAttrPayloadSize,
			"fullSize", messageFullPayloadSize,
			"error", err)
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
