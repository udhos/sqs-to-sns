// Package sqsclient provides sqs utilities.
package sqsclient

import (
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/udhos/boilerplate/awsconfig"
)

// NewClient creates an SQS client.
func NewClient(sessionName, queueURL, roleArn string) *sqs.Client {
	const me = "sqsClient"

	queueRegion, errQueue := getQueueRegion(queueURL)
	if errQueue != nil {
		log.Fatalf("%s: queue region error: %v", me, errQueue)
	}

	awsConfOptions := awsconfig.Options{
		Region:          queueRegion,
		RoleArn:         roleArn,
		RoleSessionName: sessionName,
	}

	awsConfSqs, errAwsConf := awsconfig.AwsConfig(awsConfOptions)
	if errAwsConf != nil {
		log.Fatalf("%s: aws config error: %v", me, errAwsConf)
	}

	return sqs.NewFromConfig(awsConfSqs.AwsConfig)
}

// https://sqs.us-east-1.amazonaws.com/123456789012/myqueue
func getQueueRegion(queueURL string) (string, error) {
	const me = "getQueueRegion"
	fields := strings.SplitN(queueURL, ".", 3)
	if len(fields) < 3 {
		return "", fmt.Errorf("%s: bad queue url=[%s]", me, queueURL)
	}
	region := fields[1]
	log.Printf("%s: queueRegion=[%s]", me, region)
	return region, nil
}
