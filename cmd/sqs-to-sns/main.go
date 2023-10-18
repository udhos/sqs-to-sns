// Package main implements the program.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	sns_types "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/udhos/boilerplate/boilerplate"
	"github.com/udhos/opentelemetry-trace-sqs/otelsns"
	"github.com/udhos/opentelemetry-trace-sqs/otelsqs"
	"github.com/udhos/sqs-to-sns/cmd/sqs-to-sns/internal/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const version = "1.6.7"

type applicationQueue struct {
	conf         queueConfig
	sqs          sqsClient
	sns          snsClient
	ch           chan message
	healthStatus health
	lock         sync.Mutex
}

func (q *applicationQueue) putStatus(status error) {
	h := health{status: status, when: time.Now()}
	q.lock.Lock()
	q.healthStatus = h
	q.lock.Unlock()
}

func (q *applicationQueue) getStatus() health {
	q.lock.Lock()
	h := q.healthStatus
	q.lock.Unlock()
	return h
}

type message struct {
	sqs      types.Message
	received time.Time
}

type application struct {
	cfg    config
	queues []*applicationQueue
	m      *metrics
	tracer trace.Tracer
}

func main() {

	//
	// parse cmd line
	//

	var showVersion bool
	flag.BoolVar(&showVersion, "version", showVersion, "show version")
	flag.Parse()

	//
	// show version
	//

	me := filepath.Base(os.Args[0])

	{
		v := boilerplate.LongVersion(me + " version=" + version)
		if showVersion {
			fmt.Println(v)
			return
		}
		log.Print(v)
	}

	//
	// create application
	//

	app := newApp(me, newSqsClient, newSnsClient)

	//
	// initialize tracing
	//

	{
		var tp trace.TracerProvider

		if app.cfg.jaegerEnable {
			p, errTracer := tracing.TracerProvider(me, app.cfg.jaegerURL)
			if errTracer != nil {
				log.Fatalf("tracer provider: %v", errTracer)
			}
			tp = p

			{
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				// Cleanly shutdown and flush telemetry when the application exits.
				defer func(ctx context.Context) {
					// Do not make the application hang when it is shutdown.
					ctx, cancel = context.WithTimeout(ctx, time.Second*5)
					defer cancel()
					if err := p.Shutdown(ctx); err != nil {
						log.Fatalf("trace shutdown: %v", err)
					}
				}(ctx)
			}

		} else {
			tp = trace.NewNoopTracerProvider()
		}

		// Register our TracerProvider as the global so any imported
		// instrumentation in the future will default to using it.
		otel.SetTracerProvider(tp)

		tracing.TracePropagation()

		app.tracer = tp.Tracer(fmt.Sprintf("%s-main", me))
	}

	//
	// run application
	//

	run(app)

	<-make(chan struct{}) // wait forever
}

type bogusTracer struct{}

func (t *bogusTracer) Start(ctx context.Context, _ /*spanName*/ string, _ /*opts*/ ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}

func newApp(me string, createSqsClient newSqsClientFunc, createSnsClient newSnsClientFunc) *application {
	cfg := newConfig(me)

	app := &application{
		cfg: cfg,
		m:   newMetrics(cfg.metricsNamespace, cfg.metricsBucketsLatency),

		// this bogus tracer will be replaced by actual tracer in main().
		// we assign this bogus tracer here just to prevents crashes when testing.
		tracer: &bogusTracer{},
	}

	for _, qc := range cfg.queues {
		q := &applicationQueue{
			conf: qc,
			ch:   make(chan message, qc.Buffer),
			sqs:  createSqsClient(me, qc.QueueURL, qc.QueueRoleArn, cfg.endpointURL),
			sns:  createSnsClient(me, qc.TopicArn, qc.TopicRoleArn, cfg.endpointURL),
		}
		app.queues = append(app.queues, q)
	}

	return app
}

func run(app *application) {

	if app.cfg.healthAddr != "" {
		go serveHealth(app, app.cfg.healthAddr, app.cfg.healthPath)
	}

	if app.cfg.metricsAddr != "" {
		go serveMetrics(app.cfg.metricsAddr, app.cfg.metricsPath)
	}

	for _, q := range app.queues {
		for i := 1; i <= q.conf.Readers; i++ {
			go reader(q, i, app.m)
		}
		for i := 1; i <= q.conf.Writers; i++ {
			go writer(q, i, app.m, app.tracer, app.cfg.jaegerEnable)
		}
	}
}

func reader(q *applicationQueue, readerID int, m *metrics) {

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
			q.putStatus(errRecv)
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
			if debug {
				log.Printf("%s: empty receive, sleeping %v",
					me, q.conf.EmptyReceiveCooldown)
			}
			// this cooldown prevents us from hammering the api on empty receives.
			// it shouldn't really on live aws api, but it does take place on
			// simulated apis.
			time.Sleep(q.conf.EmptyReceiveCooldown)
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

func writer(q *applicationQueue, writerID int, metric *metrics, tracer trace.Tracer, jaegerEnabled bool) {

	debug := *q.conf.Debug
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
		//m := sqsMsg.sqs

		handleMessage(me, q, sqsMsg, metric, tracer, jaegerEnabled)
	}

}

func handleMessage(me string, q *applicationQueue, sqsMsg message, metric *metrics, tracer trace.Tracer, jaegerEnabled bool) {

	debug := *q.conf.Debug
	//copyAttributes := *q.conf.CopyAttributes
	queueID := q.conf.ID
	m := &sqsMsg.sqs

	//
	// Retrieve trace context from SQS message attributes
	//
	ctx := otelsqs.ContextFromSqsMessageAttributes(m)

	ctxNew, span := tracer.Start(ctx, "handleMessage")
	defer span.End()

	if debug {
		log.Printf("%s: MessageId: %s: traceID:%v", me, *m.MessageId, span.SpanContext().TraceID().String())
		log.Printf("%s: MessageId: %s: Attributes:%v", me, *m.MessageId, toJSON(m.Attributes))
		log.Printf("%s: MessageId: %s: MessageAttributes:%v", me, *m.MessageId, toJSON(m.MessageAttributes))
		log.Printf("%s: MessageId: %s: Body:%v", me, *m.MessageId, *m.Body)
	}

	//
	// publish message to SNS topic
	//

	if published := snsPublish(ctxNew, me, q, sqsMsg, metric, tracer, jaegerEnabled); !published {
		return
	}

	// delivery latency is measured after successful publish,
	// while delivery success is recorded after successful delete
	elap := time.Since(sqsMsg.received)

	//
	// delete from source queue
	//

	if deleted := sqsDelete(ctxNew, me, q, sqsMsg, metric, tracer); !deleted {
		return
	}

	if debug {
		log.Printf("%s: sqs.DeleteMessage: %s - total sqs-to-sns latency: %v",
			me, *m.MessageId, elap)
	}

	// delivery latency is measured after successful publish,
	// while delivery success is recorded after successful delete
	q.putStatus(nil)
	metric.recordDelivery(queueID, elap)
}

func snsPublish(ctx context.Context, me string, q *applicationQueue, sqsMsg message, metric *metrics, tracer trace.Tracer, jaegerEnabled bool) bool {

	ctxNew, span := tracer.Start(ctx, "snsPublish")
	defer span.End()

	debug := *q.conf.Debug
	copyAttributes := *q.conf.CopyAttributes
	queueID := q.conf.ID
	m := &sqsMsg.sqs

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

	if jaegerEnabled {
		//
		// Inject trace context into SNS message attributes
		//
		otelsns.InjectIntoSnsMessageAttributes(ctxNew, input)
	}

	result, errPub := q.sns.Publish(context.TODO(), input)
	if errPub != nil {
		log.Printf("%s: sns.Publish: error: %v, sleeping %v",
			me, errPub, q.conf.ErrorCooldownWrite)
		metric.publishError.WithLabelValues(queueID).Inc()
		span.SetStatus(codes.Error, errPub.Error())
		time.Sleep(q.conf.ErrorCooldownWrite)
		q.putStatus(errPub)
		return false
	}

	if debug {
		log.Printf("%s: sns.Publish: %s", me, *result.MessageId)
	}

	return true
}

func sqsDelete(ctx context.Context, me string, q *applicationQueue, sqsMsg message, metric *metrics, tracer trace.Tracer) bool {

	_, span := tracer.Start(ctx, "sqsDelete")
	defer span.End()

	queueID := q.conf.ID
	m := &sqsMsg.sqs

	inputDelete := &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(q.conf.QueueURL),
		ReceiptHandle: m.ReceiptHandle,
	}

	_, errDelete := q.sqs.DeleteMessage(context.TODO(), inputDelete)
	if errDelete != nil {
		log.Printf("%s: MessageId: %s - sqs.DeleteMessage: error: %v, sleeping %v",
			me, *m.MessageId, errDelete, q.conf.ErrorCooldownDelete)
		metric.deleteError.WithLabelValues(queueID).Inc()
		time.Sleep(q.conf.ErrorCooldownDelete)
		q.putStatus(errDelete)
		return false
	}

	return true
}
