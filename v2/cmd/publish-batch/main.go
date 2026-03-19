// Package main implements the tool.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/udhos/boilerplate/awsconfig"
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

	client := newSnsClientAws(me, topicArn, roleArn, endpointURL)

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

func newSnsClientAws(sessionName, topicArn, roleArn, endpointURL string) *sns.Client {
	const me = "snsClient"

	topicRegion, errTopic := getTopicRegion(topicArn)
	if errTopic != nil {
		log.Fatalf("%s: topic region error: %v", me, errTopic)
	}

	awsConfOptions := awsconfig.Options{
		Region:          topicRegion,
		RoleArn:         roleArn,
		RoleSessionName: sessionName,
		EndpointURL:     endpointURL,
	}

	awsConf, errAwsConf := awsconfig.AwsConfig(awsConfOptions)
	if errAwsConf != nil {
		log.Fatalf("%s: aws config error: %v", me, errAwsConf)
	}

	client := sns.NewFromConfig(awsConf.AwsConfig, func(o *sns.Options) {
		if endpointURL != "" {
			o.BaseEndpoint = aws.String(endpointURL)
		}
	})

	return client
}

// arn:aws:sns:us-east-1:123456789012:mytopic
func getTopicRegion(topicArn string) (string, error) {
	const me = "getTopicRegion"
	fields := strings.SplitN(topicArn, ":", 5)
	if len(fields) < 5 {
		return "", fmt.Errorf("%s: bad topic arn=[%s]", me, topicArn)
	}
	region := fields[3]
	log.Printf("%s: topicRegion=[%s]", me, region)
	return region, nil
}
