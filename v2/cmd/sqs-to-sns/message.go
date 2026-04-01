package main

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/udhos/sqs-to-sns/v2/snsutils"
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

	m, snsPayloadBodySize, snsPayloadAttrSize := newMessageUnsafe(sqsMessage,
		receivedAt, copyAttributes, copyMessageGroupID)

	if m.snsPayloadSize > maxSnsPublishPayload {
		return message{}, fmt.Errorf("invalid payload size for SNS (body=%d, attributes=%d): total=%d > limit=%d",
			snsPayloadBodySize, snsPayloadAttrSize, m.snsPayloadSize, maxSnsPublishPayload)
	}

	return m, nil
}

func newMessageUnsafe(sqsMessage *sqstypes.Message, receivedAt time.Time,
	copyAttributes, copyMessageGroupID bool) (message, int, int) {

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

	const debug = false

	snsPayloadBodySize, snsPayloadAttrSize, snsPayloadTotalSize, _ := snsutils.GetSNSPayloadSize(snsEntry, debug)

	m := message{
		sqsMessage:     sqsMessage,
		receivedAt:     receivedAt,
		snsBatchEntry:  &snsEntry,
		snsPayloadSize: snsPayloadTotalSize,
	}

	return m, snsPayloadBodySize, snsPayloadAttrSize
}
