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
	"github.com/udhos/opentelemetry-trace-sqs/otelsqs"
	"github.com/udhos/otelconfig/oteltrace"
	"github.com/udhos/sqs-to-sns/internal/sqsclient"
	"go.opentelemetry.io/otel/trace"
)

type config struct {
	queueURL    string
	roleArn     string
	endpointURL string
	wg          sync.WaitGroup
}

const version = "1.2.0"

const batch = 10

func main() {

	me := filepath.Base(os.Args[0])

	conf := &config{}

	var count int
	var writers int
	var showVersion bool
	var debug bool
	var attributes int

	flag.IntVar(&count, "count", 30, "total number of messages to send")
	flag.IntVar(&writers, "writers", 30, "number of concurrent writers")
	flag.StringVar(&conf.queueURL, "queueURL", "", "required queue URL")
	flag.StringVar(&conf.roleArn, "roleArn", "", "optional role ARN")
	flag.StringVar(&conf.endpointURL, "endpointURL", "", "optional endpoint URL")
	flag.BoolVar(&showVersion, "version", showVersion, "show version")
	flag.BoolVar(&debug, "debug", debug, "debug")
	flag.IntVar(&attributes, "attributes", 1, "number of attributes to send")
	flag.Parse()

	{
		v := boilerplate.LongVersion(me + " version=" + version)
		if showVersion {
			fmt.Println(v)
			return
		}
		log.Print(v)
	}

	//
	// initialize tracing
	//

	var tracer trace.Tracer

	{
		options := oteltrace.TraceOptions{
			DefaultService:     me,
			NoopTracerProvider: false,
			Debug:              true,
		}

		tr, cancel, errTracer := oteltrace.TraceStart(options)

		if errTracer != nil {
			log.Fatalf("tracer: %v", errTracer)
		}

		defer cancel()

		tracer = tr
	}

	//
	// send
	//

	ctx, span := tracer.Start(context.TODO(), me)
	defer span.End()

	traceID := span.SpanContext().TraceID().String()

	messages := []types.SendMessageBatchRequestEntry{}

	body := fmt.Sprintf("batch-sqs traceID:%s", traceID)

	for i := 0; i < batch; i++ {
		id := strconv.Itoa(i)
		m := types.SendMessageBatchRequestEntry{
			MessageBody:       aws.String(body),
			Id:                aws.String(id),
			MessageAttributes: make(map[string]types.MessageAttributeValue),
		}

		for j := 0; j < attributes; j++ {
			str := fmt.Sprintf("%d", j)
			m.MessageAttributes[str] = types.MessageAttributeValue{
				StringValue: aws.String(str),
				DataType:    aws.String("String"),
			}
		}

		if debug {
			log.Printf("MessageId:%s TraceId:%s", aws.ToString(m.Id), traceID)
		}

		otelsqs.NewCarrier().Inject(ctx, m.MessageAttributes)

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

	sqsClient := sqsclient.NewClient(me, conf.queueURL, conf.roleArn, conf.endpointURL)

	for sent := 0; sent < count; {
		input := &sqs.SendMessageBatchInput{
			Entries:  messages,
			QueueUrl: &conf.queueURL,
		}
		output, errSend := sqsClient.SendMessageBatch(context.TODO(), input)
		if errSend != nil {
			log.Printf("%s: SendMessageBatch error: %v, sleeping %v",
				me, errSend, cooldown)
			time.Sleep(cooldown)
			continue
		}
		for _, f := range output.Failed {
			log.Printf("%s: message failed: id=%s sender_fault=%t code=%s message:%s",
				me, aws.ToString(f.Id), f.SenderFault, aws.ToString(f.Code), aws.ToString(f.Message))
		}
		sent += batch
	}

	elap := time.Since(begin)

	rate := float64(count) / float64(elap/time.Second)

	log.Printf("%s: sent=%d interval=%v rate=%v messages/sec",
		me, count, elap, rate)
}
