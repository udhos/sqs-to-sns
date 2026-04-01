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
	body = stringByteSize(aws.ToString(snsEntry.Message))

	attrs := snsEntry.MessageAttributes

	for name, attr := range attrs {
		// 1. Name of the attribute
		attributes += stringByteSize(name)

		// 2. DataType (e.g., "String", "Number", "Binary")
		if attr.DataType != nil {
			attributes += stringByteSize(aws.ToString(attr.DataType))
		}

		// 3. String Value
		if attr.StringValue != nil {
			attributes += stringByteSize(aws.ToString(attr.StringValue))
		}

		// 4. Binary Value
		if len(attr.BinaryValue) > 0 {
			attributes += stringByteSize(string(attr.BinaryValue))
		}
	}

	total = body + attributes

	return
}

// stringByteSize calculates the byte size of a string, accounting for UTF-8 encoding.
func stringByteSize(s string) int {
	return len([]byte(s))
}
