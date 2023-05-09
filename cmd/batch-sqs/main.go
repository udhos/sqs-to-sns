// Package main implements an utility.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/udhos/boilerplate/boilerplate"
	"github.com/udhos/sqs-to-sns/sqsclient"
)

type config struct {
	queueURL string
	roleArn  string
	wg       sync.WaitGroup
}

const version = "0.1.0"

const batch = 10

func main() {

	me := filepath.Base(os.Args[0])

	conf := &config{}

	var count int
	var writers int
	var showVersion bool

	flag.IntVar(&count, "count", 10000, "total number of messages to send")
	flag.IntVar(&writers, "writers", 30, "number of concurrent writers")
	flag.StringVar(&conf.queueURL, "queueURL", "", "required queue URL")
	flag.StringVar(&conf.roleArn, "roleArn", "", "optional role ARN")
	flag.BoolVar(&showVersion, "version", showVersion, "show version")
	flag.Parse()

	{
		v := boilerplate.LongVersion(me + " version=" + version)
		if showVersion {
			fmt.Println(v)
			return
		}
		log.Print(v)
	}

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

	begin := time.Now()

	for i := 1; i <= writers; i++ {
		conf.wg.Add(1)
		go writer(i, writers, conf, count/writers, messages)
	}

	conf.wg.Wait()

	elap := time.Since(begin)

	rate := float64(count) / (float64(elap) / float64(time.Second))

	log.Printf("%s: sent=%d interval=%v rate=%v messages/sec",
		me, count, elap, rate)
}

func writer(id, total int, conf *config, count int, messages []types.SendMessageBatchRequestEntry) {
	defer conf.wg.Done()

	me := fmt.Sprintf("writer: [%d/%d]", id, total)

	log.Printf("%s: will send %d", me, count)

	begin := time.Now()

	const cooldown = 5 * time.Second

	sqsClient := sqsclient.NewClient(me, conf.queueURL, conf.roleArn)

	for sent := 0; sent < count; {
		input := &sqs.SendMessageBatchInput{
			Entries:  messages,
			QueueUrl: &conf.queueURL,
		}
		_, errSend := sqsClient.SendMessageBatch(context.TODO(), input)
		if errSend != nil {
			log.Printf("%s: SendMessageBatch error: %v, sleeping %v",
				me, errSend, cooldown)
			time.Sleep(cooldown)
			continue
		}
		sent += batch
	}

	elap := time.Since(begin)

	rate := float64(count) / float64(elap/time.Second)

	log.Printf("%s: sent=%d interval=%v rate=%v messages/sec",
		me, count, elap, rate)
}
