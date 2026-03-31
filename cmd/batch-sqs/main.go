// Package main implements an utility.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

type flagURL struct {
	urls []string
}

func (u *flagURL) String() string {
	return strings.Join(u.urls, ",")
}

func (u *flagURL) Set(value string) error {
	u.urls = append(u.urls, value)
	return nil
}

type config struct {
	queueURL    flagURL
	roleArn     string
	endpointURL string
	wg          sync.WaitGroup
}

const version = "1.4.0"

const batch = 10

const snsPayloadLimit = 262144

func main() {

	me := filepath.Base(os.Args[0])

	conf := &config{}

	var count int
	var writers int
	var showVersion bool
	var debug bool
	var attributes int
	var sizeMin int
	var sizeMax int
	var batchSizeLimit int
	var otel bool
	var errorCooldown time.Duration

	flag.IntVar(&count, "count", 20, "total number of messages to send")
	flag.IntVar(&writers, "writers", 2, "number of concurrent writers")
	flag.Var(&conf.queueURL, "queueURL", "required queue URL (repeat for multiple queues)")
	flag.StringVar(&conf.roleArn, "roleArn", "", "optional role ARN")
	flag.StringVar(&conf.endpointURL, "endpointURL", "", "optional endpoint URL")
	flag.BoolVar(&showVersion, "version", false, "show version")
	flag.BoolVar(&debug, "debug", false, "debug")
	flag.IntVar(&attributes, "attributes", 0, "number of attributes to send")
	flag.IntVar(&sizeMin, "sizeMin", 10000, "min for random message size")
	flag.IntVar(&sizeMax, "sizeMax", 10000, "max for random message size")
	flag.IntVar(&batchSizeLimit, "batchSizeLimit", snsPayloadLimit/10,
		"if message random size hits this size it will be sent alone in a single message batch")
	flag.BoolVar(&otel, "otel", false, "otel trace")
	flag.DurationVar(&errorCooldown, "errorCooldown", 5*time.Second, "cooldown time after error before retrying")
	flag.Parse()

	{
		v := boilerplate.LongVersion(me + " version=" + version)
		if showVersion {
			fmt.Println(v)
			return
		}
		log.Print(v)
	}

	log.Printf("queues: %s", conf.queueURL.String())
	log.Printf("sizeMin=%d sizeMax=%d batchSizeLimit=%d", sizeMax, sizeMax, batchSizeLimit)

	if sizeMax < sizeMin {
		log.Fatalf("sizeMax=%d < sizeMin=%d", sizeMax, sizeMin)
	}

	//
	// initialize tracing
	//

	var tracer trace.Tracer

	{
		options := oteltrace.TraceOptions{
			DefaultService:     me,
			NoopTracerProvider: !otel,
			Debug:              debug,
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

	opt := options{
		ctx:            ctx,
		sizeMin:        sizeMin,
		sizeMax:        sizeMax,
		batchSizeLimit: batchSizeLimit,
		attributes:     attributes,
		traceID:        traceID,
		debug:          debug,
		otel:           otel,
		errorCooldown:  errorCooldown,
	}

	begin := time.Now()

	for i := 1; i <= writers; i++ {
		conf.wg.Add(1)
		go writer(i, writers, conf, count/writers, opt)
	}

	conf.wg.Wait()

	elap := time.Since(begin)

	rate := float64(count) / (float64(elap) / float64(time.Second))

	log.Printf("%s: sent=%d interval=%v rate=%f messages/sec",
		me, count, elap, rate)
}

type options struct {
	ctx            context.Context
	sizeMin        int
	sizeMax        int
	batchSizeLimit int
	attributes     int
	traceID        string
	debug          bool
	otel           bool
	errorCooldown  time.Duration
}

func generateBatch(opt options) []types.SendMessageBatchRequestEntry {

	messages := []types.SendMessageBatchRequestEntry{}

	bodySize := rand.Intn(1+opt.sizeMax-opt.sizeMin) + opt.sizeMin

	if opt.debug {
		log.Printf("bodySize: %d", bodySize)
	}

	body := strings.Repeat("a", bodySize)

	for i := range batch {
		id := strconv.Itoa(i)
		m := types.SendMessageBatchRequestEntry{
			MessageBody:       aws.String(body),
			Id:                aws.String(id),
			MessageAttributes: make(map[string]types.MessageAttributeValue),
		}

		for j := range opt.attributes {
			str := fmt.Sprintf("%d", j)
			m.MessageAttributes[str] = types.MessageAttributeValue{
				StringValue: aws.String(str),
				DataType:    aws.String("String"),
			}
		}

		if opt.debug {
			log.Printf("MessageId:%s TraceId:%s", aws.ToString(m.Id), opt.traceID)
		}

		if opt.otel {
			otelsqs.NewCarrier().Inject(opt.ctx, m.MessageAttributes)
		}

		messages = append(messages, m)

		if bodySize >= opt.batchSizeLimit {
			break // send as single message batch
		}
	}

	return messages
}

func writer(id, total int, conf *config, messagesToSendPerWriter int, opt options) {
	defer conf.wg.Done()

	me := fmt.Sprintf("writer: [%d/%d]", id, total)

	log.Printf("%s: will send %d", me, messagesToSendPerWriter)

	begin := time.Now()

	// create sqs clients
	var sqsClients []*sqs.Client
	for _, queueURL := range conf.queueURL.urls {
		sqsClient := sqsclient.NewClient(me, queueURL, conf.roleArn, conf.endpointURL)
		sqsClients = append(sqsClients, sqsClient)
	}

	var sent int
	var errors int

	for sent < messagesToSendPerWriter {

		messages := generateBatch(opt)

		for i, queueURL := range conf.queueURL.urls {

			output, errSend := send(sqsClients[i], queueURL, messages)
			if errSend != nil {
				errors++
				log.Printf("%s: SendMessageBatch error %d: %v, sleeping cooldown=%v",
					me, errors, errSend, opt.errorCooldown)
				time.Sleep(opt.errorCooldown)
				continue
			}
			for _, f := range output.Failed {
				log.Printf("%s: message failed: id=%s sender_fault=%t code=%s message:%s",
					me, aws.ToString(f.Id), f.SenderFault, aws.ToString(f.Code), aws.ToString(f.Message))
			}
		}

		sent += len(messages)
	}

	elap := time.Since(begin)

	rate := float64(sent) / (float64(elap) / float64(time.Second))

	log.Printf("%s: sent=%d interval=%v rate=%f messages/sec errors=%d",
		me, messagesToSendPerWriter, elap, rate, errors)
}

func send(client *sqs.Client,
	queueURL string,
	messages []types.SendMessageBatchRequestEntry) (*sqs.SendMessageBatchOutput, error) {

	input := &sqs.SendMessageBatchInput{
		Entries:  messages,
		QueueUrl: aws.String(queueURL),
	}

	return client.SendMessageBatch(context.TODO(), input)
}
