package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/udhos/boilerplate/awsconfig"
)

type snsClient interface {
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

func newSnsClient(sessionName, topicArn, roleArn string) snsClient {
	return newSnsClientAws(sessionName, topicArn, roleArn) // create real sns client
}

type newSnsClientFunc func(sessionName, topicArn, roleArn string) snsClient

func newSnsClientAws(sessionName, topicArn, roleArn string) *sns.Client {
	const me = "snsClient"

	topicRegion, errTopic := getTopicRegion(topicArn)
	if errTopic != nil {
		log.Fatalf("%s: topic region error: %v", me, errTopic)
	}

	awsConfOptions := awsconfig.Options{
		Region:          topicRegion,
		RoleArn:         roleArn,
		RoleSessionName: sessionName,
	}

	awsConf, errAwsConf := awsconfig.AwsConfig(awsConfOptions)
	if errAwsConf != nil {
		log.Fatalf("%s: aws config error: %v", me, errAwsConf)
	}

	return sns.NewFromConfig(awsConf.AwsConfig)
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
