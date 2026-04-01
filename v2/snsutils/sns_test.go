package snsutils

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
)

// go test -count 1 -run '^TestGetSNSPayloadSize$' ./...
func TestGetSNSPayloadSize(t *testing.T) {
	tests := []struct {
		name           string
		entry          snstypes.PublishBatchRequestEntry
		wantBody       int
		wantAttributes int
		wantTotal      int
	}{
		{
			name: "Empty Entry",
			entry: snstypes.PublishBatchRequestEntry{
				Message: aws.String(""),
			},
			wantBody:       0,
			wantAttributes: 0,
			wantTotal:      0,
		},
		{
			name: "Only Message Body",
			entry: snstypes.PublishBatchRequestEntry{
				Message: aws.String("Hello World"),
			},
			wantBody:       11,
			wantAttributes: 0,
			wantTotal:      11,
		},
		{
			name: "UTF-8 Multi-byte Characters",
			entry: snstypes.PublishBatchRequestEntry{
				Message: aws.String("🚀"), // 4 bytes
			},
			wantBody:       4,
			wantAttributes: 0,
			wantTotal:      4,
		},
		{
			name: "Single String Attribute",
			entry: snstypes.PublishBatchRequestEntry{
				Message: aws.String("msg"),
				MessageAttributes: map[string]snstypes.MessageAttributeValue{
					"test-key": {
						DataType:    aws.String("String"),
						StringValue: aws.String("test-value"),
					},
				},
			},
			// key (8) + DataType (6) + StringValue (10) = 24
			wantBody:       3,
			wantAttributes: 24,
			wantTotal:      27,
		},
		{
			name: "Binary Attribute",
			entry: snstypes.PublishBatchRequestEntry{
				Message: aws.String("msg"),
				MessageAttributes: map[string]snstypes.MessageAttributeValue{
					"bin-key": {
						DataType:    aws.String("Binary"),
						BinaryValue: []byte{0x01, 0x02, 0x03},
					},
				},
			},
			// key (7) + DataType (6) + BinaryValue (3) = 16
			wantBody:       3,
			wantAttributes: 16,
			wantTotal:      19,
		},
		{
			name: "Multiple Attributes",
			entry: snstypes.PublishBatchRequestEntry{
				Message: aws.String("A"),
				MessageAttributes: map[string]snstypes.MessageAttributeValue{
					"k1": {DataType: aws.String("S"), StringValue: aws.String("V")}, // 2 + 1 + 1 = 4
					"k2": {DataType: aws.String("S"), StringValue: aws.String("V")}, // 2 + 1 + 1 = 4
				},
			},
			wantBody:       1,
			wantAttributes: 8,
			wantTotal:      9,
		},
		{
			name: "Large 250KB Message",
			entry: snstypes.PublishBatchRequestEntry{
				Message: aws.String(strings.Repeat("a", 250*1024)),
			},
			wantBody:       256000,
			wantAttributes: 0,
			wantTotal:      256000,
		},
		{
			name: "Maximum SNS Size (256KB)",
			entry: snstypes.PublishBatchRequestEntry{
				// strings.Repeat is efficient for generating large test strings
				Message: aws.String(strings.Repeat("a", 262144)),
			},
			wantBody:       262144,
			wantAttributes: 0,
			wantTotal:      262144,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const debug = true
			gotBody, gotAttributes, gotTotal, debugInfo := GetSNSPayloadSize(tt.entry, debug)

			if gotBody != tt.wantBody {
				t.Errorf("GetSNSPayloadSize() gotBody = %v, want %v, debugInfo: %s",
					gotBody, tt.wantBody, debugInfo)
			}
			if gotAttributes != tt.wantAttributes {
				t.Errorf("GetSNSPayloadSize() gotAttributes = %v, want %v, debugInfo: %s",
					gotAttributes, tt.wantAttributes, debugInfo)
			}
			if gotTotal != tt.wantTotal {
				t.Errorf("GetSNSPayloadSize() gotTotal = %v, want %v, debugInfo: %s",
					gotTotal, tt.wantTotal, debugInfo)
			}
		})
	}
}
