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
	"github.com/udhos/sqs-to-sns/sqsclient"
)

const version = "0.7.0"

func getVersion(me string) string {
	return fmt.Sprintf("%s version=%s runtime=%s boilerplate=%s GOOS=%s GOARCH=%s GOMAXPROCS=%d",
		me, version, runtime.Version(), boilerplate.Version(), runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0))
}

type applicationQueue struct {
	conf queueConfig
	sqs  *sqs.Client
	sns  *sns.Client
	ch   chan message
}

type message struct {
	sqs      types.Message
	received time.Time
}

type application struct {
	queues []applicationQueue
	m      *metrics
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
		m: newMetrics(cfg.metricsNamespace),
	}

	for _, qc := range cfg.queues {
		q := applicationQueue{
			conf: qc,
			ch:   make(chan message, qc.Buffer),
			sqs:  sqsclient.NewClient(me, qc.QueueURL, qc.QueueRoleArn),
			sns:  snsClient(me, qc.TopicArn, qc.TopicRoleArn),
		}
		app.queues = append(app.queues, q)
	}

	go serveMetrics(cfg.metricsAddr, cfg.metricsPath)

	run(app)
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

	for _, q := range app.queues {

		for i := 1; i <= q.conf.Readers; i++ {
			go reader(q, i, app.m)
		}

		for i := 1; i <= q.conf.Writers; i++ {
			go writer(q, i, app.m)
		}

	}

	<-make(chan struct{}) // wait forever
}

func reader(q applicationQueue, readerID int, m *metrics) {

	debug := *q.conf.Debug

	queueID := q.conf.ID

	me := fmt.Sprintf("reader %s[%d/%d]", queueID, readerID, q.conf.Readers)

	input := &sqs.ReceiveMessageInput{
		QueueUrl: aws.String(q.conf.QueueURL),
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
		if debug {
			log.Printf("%s: ready: %s", me, q.conf.QueueURL)
		}

		//
		// read message from sqs queue
		//

		m.receive.WithLabelValues(queueID).Inc()

		resp, errRecv := q.sqs.ReceiveMessage(context.TODO(), input)
		if errRecv != nil {
			log.Printf("%s: sqs.ReceiveMessage: error: %v, sleeping %v",
				me, errRecv, q.conf.ErrorCooldownRead)
			m.receiveError.WithLabelValues(queueID).Inc()
			time.Sleep(q.conf.ErrorCooldownRead)
			continue
		}

		//
		// push messages into channel
		//

		count := len(resp.Messages)

		if debug {
			log.Printf("%s: sqs.ReceiveMessage: found %d messages", me, count)
		}

		if count == 0 {
			m.receiveEmpty.WithLabelValues(queueID).Inc()
			continue
		}

		m.receiveMessages.WithLabelValues(queueID).Add(float64(count))

		for i, msg := range resp.Messages {
			if debug {
				log.Printf("%s: %d/%d MessageId: %s", me, i+1, count, *msg.MessageId)
			}
			q.ch <- message{sqs: msg, received: time.Now()}
			m.buffer.WithLabelValues(queueID).Inc()
		}
	}

}

func writer(q applicationQueue, writerID int, metric *metrics) {

	debug := *q.conf.Debug
        copyAttr := *q.conf.CopyAttributes

	queueID := q.conf.ID

	me := fmt.Sprintf("writer %s[%d/%d]", queueID, writerID, q.conf.Writers)

	for {
		if debug {
			log.Printf("%s: ready: %s", me, q.conf.TopicArn)
		}

		//
		// read message from channel
		//

		sqsMsg := <-q.ch
		metric.buffer.WithLabelValues(queueID).Dec()
		m := sqsMsg.sqs

		if debug {
			log.Printf("%s: MessageId: %s: Attributes:%v", me, *m.MessageId, toJSON(m.Attributes))
			log.Printf("%s: MessageId: %s: MessageAttributes:%v", me, *m.MessageId, toJSON(m.MessageAttributes))
			log.Printf("%s: MessageId: %s: Body:%v", me, *m.MessageId, *m.Body)
		}

		//
		// publish message to sns topic
		//

		input := &sns.PublishInput{
			Message:  m.Body,
			TopicArn: aws.String(q.conf.TopicArn),
		}

		if copyAttributes {
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

		result, errPub := q.sns.Publish(context.TODO(), input)
		if errPub != nil {
			log.Printf("%s: sns.Publish: error: %v, sleeping %v",
				me, errPub, q.conf.ErrorCooldownWrite)
			metric.publishError.WithLabelValues(queueID).Inc()
			time.Sleep(q.conf.ErrorCooldownWrite)
			continue
		}

		if debug {
			log.Printf("%s: sns.Publish: %s", me, *result.MessageId)
		}

		//
		// delete from source queue
		//

		inputDelete := &sqs.DeleteMessageInput{
			QueueUrl:      aws.String(q.conf.QueueURL),
			ReceiptHandle: m.ReceiptHandle,
		}

		_, errDelete := q.sqs.DeleteMessage(context.TODO(), inputDelete)
		if errDelete != nil {
			log.Printf("%s: MessageId: %s - sqs.DeleteMessage: error: %v", me, *m.MessageId, errDelete)
			metric.deleteError.WithLabelValues(queueID).Inc()
			continue
		}

		elap := time.Since(sqsMsg.received)

		if debug {
			log.Printf("%s: sqs.DeleteMessage: %s - total sqs-to-sns latency: %v",
				me, *m.MessageId, elap)
		}

		metric.recordDelivery(queueID, elap)
	}

}
