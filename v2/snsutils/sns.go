// Package snsutils provides utility functions for working with AWS SNS (Simple Notification Service).
package snsutils

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
)

// GetSNSPayloadSize calculates the total payload size of an SNS PublishBatchRequestEntry,
// including the message body and all message attributes.
// It returns the size of the message body, the total size of all attributes, and the combined total size.
func GetSNSPayloadSize(snsEntry snstypes.PublishBatchRequestEntry) (body, attributes, total int) {
	body = awsStringByteSize(snsEntry.Message)

	attrs := snsEntry.MessageAttributes

	for name, attr := range attrs {
		// 1. Name of the attribute
		attributes += len(name)

		// 2. DataType (e.g., "String", "Number", "Binary")
		if attr.DataType != nil {
			attributes += awsStringByteSize(attr.DataType)
		}

		// 3. String Value
		if attr.StringValue != nil {
			attributes += awsStringByteSize(attr.StringValue)
		}

		// 4. Binary Value
		if len(attr.BinaryValue) > 0 {
			attributes += len(attr.BinaryValue)
		}
	}

	total = body + attributes

	return
}

func awsStringByteSize(s *string) int {
	return len(aws.ToString(s))
}
