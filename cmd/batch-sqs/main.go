// Package main implements an utility.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/udhos/sqs-to-sns/sqsclient"
)

func main() {

	me := filepath.Base(os.Args[0])

	var count int
	var queueURL string
	var roleArn string

	flag.IntVar(&count, "count", 10, "number of messages")
	flag.StringVar(&queueURL, "queueURL", "", "required queue URL")
	flag.StringVar(&roleArn, "roleArn", "", "optional role ARN")
	flag.Parse()

	sqsClient := sqsclient.NewClient(me, queueURL, roleArn)

	const batch = 10
	const cooldown = 5 * time.Second

	messages := []types.SendMessageBatchRequestEntry{}

	body := "hello world"

	for i := 0; i < batch; i++ {
		id := strconv.Itoa(i)
		m := types.SendMessageBatchRequestEntry{
			MessageBody: aws.String(body),
			Id:          aws.String(id),
		}
		messages = append(messages, m)
	}

	for sent := 0; sent < count; {
		input := &sqs.SendMessageBatchInput{
			Entries:  messages,
			QueueUrl: &queueURL,
		}
		_, errSend := sqsClient.SendMessageBatch(context.TODO(), input)
		if errSend != nil {
			log.Printf("%s: SendMessageBatch error: %v, sleeping %v",
				me, errSend, cooldown)
			time.Sleep(cooldown)
			continue
		}
		sent += batch
		log.Printf("%s: sent: %d/%d", me, sent, count)
	}
}
