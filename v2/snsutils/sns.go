// Package snsutils provides utility functions for working with AWS SNS (Simple Notification Service).
package snsutils

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
)

// GetSNSPayloadSize calculates the total payload size of an SNS PublishBatchRequestEntry,
// including the message body and all message attributes.
// It returns the size of the message body, the total size of all attributes, and the combined total size.
func GetSNSPayloadSize(snsEntry snstypes.PublishBatchRequestEntry,
	debug bool) (body, attributes, total int, debugInfo string) {

	body = awsStringByteSize(snsEntry.Message)

	attrs := snsEntry.MessageAttributes

	var debugAttr []string

	for name, attr := range attrs {
		// 1. Name of the attribute
		nameLen := len(name)
		attributes += nameLen
		if debug {
			debugAttr = append(debugAttr, fmt.Sprintf("attrName=%d:%q", nameLen, name))
		}

		// 2. DataType (e.g., "String", "Number", "Binary")
		if attr.DataType != nil {
			dataTypeLen := awsStringByteSize(attr.DataType)
			attributes += dataTypeLen
			if debug {
				debugAttr = append(debugAttr, fmt.Sprintf("attrDataType=%d:%q", dataTypeLen, aws.ToString(attr.DataType)))
			}
		}

		// 3. String Value
		if attr.StringValue != nil {
			stringValueLen := awsStringByteSize(attr.StringValue)
			attributes += stringValueLen
			if debug {
				debugAttr = append(debugAttr, fmt.Sprintf("attrStringValue=%d:%q", stringValueLen, aws.ToString(attr.StringValue)))
			}
		}

		// 4. Binary Value
		if len(attr.BinaryValue) > 0 {
			binaryValueLen := len(attr.BinaryValue)
			attributes += binaryValueLen
			if debug {
				debugAttr = append(debugAttr, fmt.Sprintf("attrBinaryValue=%d:%q", binaryValueLen, string(attr.BinaryValue)))
			}
		}
	}

	total = body + attributes

	if debug {
		debugInfo = strings.Join(debugAttr, " ")
	}

	return
}

func awsStringByteSize(s *string) int {
	return len(aws.ToString(s))
}
