package main

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

const maxSnsPublishPayload = 262144

type message struct {
	sqsMessage     *sqstypes.Message
	receivedAt     time.Time
	snsBatchEntry  *snstypes.PublishBatchRequestEntry
	snsPayloadSize int
}

func newMessage(sqsMessage *sqstypes.Message, receivedAt time.Time,
	copyAttributes, copyMessageGroupID bool) (message, error) {

	snsEntry := snstypes.PublishBatchRequestEntry{
		Message: sqsMessage.Body,
	}

	if copyAttributes {
		//
		// copy attributes from SQS to SNS
		//
		attr := map[string]snstypes.MessageAttributeValue{}
		for k, v := range sqsMessage.MessageAttributes {
			attr[k] = snstypes.MessageAttributeValue{
				DataType:    v.DataType,
				BinaryValue: v.BinaryValue,
				StringValue: v.StringValue,
			}
		}
		snsEntry.MessageAttributes = attr
	}

	if copyMessageGroupID {
		//
		// copy message group id from SQS to SNS
		//
		if messageGroupID := sqsMessage.Attributes["MessageGroupId"]; messageGroupID != "" {
			snsEntry.MessageGroupId = aws.String(messageGroupID)
		}
	}

	m := message{
		sqsMessage:     sqsMessage,
		receivedAt:     receivedAt,
		snsBatchEntry:  &snsEntry,
		snsPayloadSize: getSNSPayloadSize(len(aws.ToString(snsEntry.Message)), snsEntry.MessageAttributes),
	}

	if m.snsPayloadSize > maxSnsPublishPayload {
		return message{}, fmt.Errorf("invalid payload size for SNS: %d > limit=%d",
			m.snsPayloadSize, maxSnsPublishPayload)
	}

	return m, nil
}

func getSNSPayloadSize(messageSize int, attrs map[string]snstypes.MessageAttributeValue) int {

	for name, attr := range attrs {
		// 1. Name of the attribute
		messageSize += len(name)

		// 2. DataType (e.g., "String", "Number", "Binary")
		if attr.DataType != nil {
			messageSize += len(aws.ToString(attr.DataType))
		}

		// 3. String Value
		if attr.StringValue != nil {
			messageSize += len(aws.ToString(attr.StringValue))
		}

		// 4. Binary Value
		if len(attr.BinaryValue) > 0 {
			messageSize += len(attr.BinaryValue)
		}
	}

	return messageSize
}
