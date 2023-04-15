// Package main implements the program.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	sns_types "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/udhos/boilerplate/awsconfig"
	"github.com/udhos/boilerplate/boilerplate"
)

const version = "0.1.0"

func getVersion(me string) string {
	return fmt.Sprintf("%s version=%s runtime=%s boilerplate=%s GOOS=%s GOARCH=%s GOMAXPROCS=%d",
		me, version, runtime.Version(), boilerplate.Version(), runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0))
}

type application struct {
	conf config
	sqs  *sqs.Client
	sns  *sns.Client
	ch   chan types.Message
}

func main() {

	var showVersion bool
	flag.BoolVar(&showVersion, "version", showVersion, "show version")
	flag.Parse()

	me := filepath.Base(os.Args[0])

	{
		v := getVersion(me)
		if showVersion {
			fmt.Print(v)
			fmt.Println()
			return
		}
		log.Print(v)
	}

	cfg := newConfig(me)

	app := &application{
		conf: cfg,
		ch:   make(chan types.Message, cfg.buffer),
	}

	app.sqs = sqsClient(me, cfg.queueURL, cfg.roleArnSqs)

	app.sns = snsClient(me, cfg.topicArn, cfg.roleArnSns)

	run(app)
}

func sqsClient(sessionName, queueURL, roleArn string) *sqs.Client {
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

func snsClient(sessionName, topicArn, roleArn string) *sns.Client {
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

func run(app *application) {

	for i := 0; i < app.conf.readers; i++ {
		go reader(i, app)
	}

	for i := 0; i < app.conf.writers; i++ {
		go writer(i, app)
	}

	<-make(chan struct{}) // wait forever
}

func reader(id int, app *application) {

	me := fmt.Sprintf("reader[%d]", id)

	input := &sqs.ReceiveMessageInput{
		QueueUrl: aws.String(app.conf.queueURL),
		AttributeNames: []types.QueueAttributeName{
			"SentTimestamp",
		},
		MaxNumberOfMessages: 10, // 1..10
		MessageAttributeNames: []string{
			"All",
		},
		WaitTimeSeconds: 20, // 0..20
	}

	for {
		log.Printf("%s: ready: %s", me, app.conf.queueURL)

		resp, errRecv := app.sqs.ReceiveMessage(context.TODO(), input)
		if errRecv != nil {
			log.Printf("%s: sqs.ReceiveMessage: error: %v, sleeping %v",
				me, errRecv, app.conf.errorCooldownRead)
			time.Sleep(app.conf.errorCooldownRead)
			continue
		}

		count := len(resp.Messages)

		log.Printf("%s: sqs.ReceiveMessage: found %d messages", me, count)

		for i, msg := range resp.Messages {
			log.Printf("%s: %d/%d MessageId: %s", me, i+1, count, *msg.MessageId)
			app.ch <- msg
		}
	}

}

func writer(id int, app *application) {

	me := fmt.Sprintf("writer[%d]", id)

	for {
		log.Printf("%s: ready: %s", me, app.conf.topicArn)

		//
		// read message from channel
		//

		m := <-app.ch
		log.Printf("%s: MessageId: %s", me, *m.MessageId)

		if app.conf.debug {
			log.Printf("%s: MessageId: %s: Attributes:%v", me, *m.MessageId, m.Attributes)
			log.Printf("%s: MessageId: %s: MessageAttributes:%v", me, *m.MessageId, m.MessageAttributes)
			log.Printf("%s: MessageId: %s: Body:%v", me, *m.MessageId, *m.Body)
		}

		if app.conf.copyAttributes {

		}

		//
		// publish message to sns topic
		//

		input := &sns.PublishInput{
			Message:  m.Body,
			TopicArn: aws.String(app.conf.topicArn),
		}

		if app.conf.copyAttributes {
			//
			// copy attributes from SQS to SNS
			//
			attr := map[string]sns_types.MessageAttributeValue{}
			for k, v := range m.MessageAttributes {
				attr[k] = sns_types.MessageAttributeValue{
					DataType:    v.DataType,
					BinaryValue: v.BinaryValue,
					StringValue: v.StringValue,
				}
			}
			input.MessageAttributes = attr
		}

		result, err := app.sns.Publish(context.TODO(), input)
		if err != nil {
			log.Printf("%s: sns.Publish: error: %v, sleeping %v",
				me, err, app.conf.errorCooldownWrite)
			time.Sleep(app.conf.errorCooldownWrite)
			continue
		}

		log.Printf("%s: sns.Publish: %s", me, *result.MessageId)

		//
		// delete from source queue
		//

		inputDelete := &sqs.DeleteMessageInput{
			QueueUrl:      aws.String(app.conf.queueURL),
			ReceiptHandle: m.ReceiptHandle,
		}

		_, errDelete := app.sqs.DeleteMessage(context.TODO(), inputDelete)
		if errDelete != nil {
			log.Printf("%s: MessageId: %s - sqs.DeleteMessage: error: %v", me, *m.MessageId, errDelete)
			continue
		}

		log.Printf("%s: sqs.DeleteMessage: %s", me, *m.MessageId)
	}

}
