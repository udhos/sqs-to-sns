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

	_ "github.com/KimMachineGun/automemlimit"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	sns_types "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/udhos/boilerplate/boilerplate"
	"github.com/udhos/opentelemetry-trace-sqs/otelsns"
	"github.com/udhos/opentelemetry-trace-sqs/otelsqs"
	"github.com/udhos/otelconfig/oteltrace"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	_ "go.uber.org/automaxprocs"
)

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
	prom   *prom
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

	if app.cfg.jaegerEnable {
		options := oteltrace.TraceOptions{
			DefaultService:     me,
			NoopTracerProvider: !app.cfg.jaegerEnable,
			Debug:              true,
		}

		tracer, cancel, errTracer := oteltrace.TraceStart(options)

		if errTracer != nil {
			log.Fatalf("tracer: %v", errTracer)
		}

		defer cancel()

		app.tracer = tracer
	} else {
		app.tracer = oteltrace.NewNoopTracer()
	}

	//
	// run application
	//

	run(app)

	<-make(chan struct{}) // wait forever
}

func newApp(me string, createSqsClient newSqsClientFunc, createSnsClient newSnsClientFunc) *application {
	cfg := newConfig(me)

	prom := newProm()

	app := &application{
		cfg: cfg,
		m: newMetrics(prom, cfg.dogstatsdEnable, cfg.dogstatsdDebug,
			cfg.metricsNamespace, cfg.metricsBucketsLatency),

		// this bogus tracer will be replaced by actual tracer in main().
		// we assign this bogus tracer here just to prevents crashes when testing.
		tracer: &noop.Tracer{},

		prom: prom,
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
		serveHealth(app, app.cfg.healthAddr, app.cfg.healthPath)
	}

	if app.cfg.prometheusEnable {
		if app.cfg.metricsAddr != "" {
			go serveMetrics(app.prom, app.cfg.metricsAddr, app.cfg.metricsPath)
		}
	}

	for _, q := range app.queues {
		for i := 1; i <= q.conf.Readers; i++ {
			go reader(q, i, app.m)
		}
		for i := 1; i <= q.conf.Writers; i++ {
			go writer(q, i, app.m, app.tracer, app.cfg.jaegerEnable,
				app.cfg.ignoreSqsAttributeLimit)
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
		MaxNumberOfMessages: *q.conf.MaxNumberOfMessages, // 1..10 (default 10)
		MessageAttributeNames: []string{
			"All",
		},
		WaitTimeSeconds: *q.conf.WaitTimeSeconds, // 0..20 (default 20)
	}

	for {
		if debug {
			log.Printf("%s: ready: %s MaxNumberOfMessages=%d WaitTimeSeconds=%d",
				me, q.conf.QueueURL, input.MaxNumberOfMessages, input.WaitTimeSeconds)
		}

		//
		// read message from sqs queue
		//

		m.incReceive(queueID)

		resp, errRecv := q.sqs.ReceiveMessage(context.TODO(), input)
		if errRecv != nil {
			log.Printf("%s: sqs.ReceiveMessage: error: %v, sleeping %v",
				me, errRecv, q.conf.ErrorCooldownRead)
			m.incReceiveError(queueID)
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
			m.incReceiveEmpty(queueID)
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

		m.incReceiveMessages(queueID, float64(count))

		for i, msg := range resp.Messages {
			if debug {
				log.Printf("%s: %d/%d MessageId: %s", me, i+1, count, *msg.MessageId)
			}
			q.ch <- message{sqs: msg, received: time.Now()}
			m.gaugeBuffer(queueID, float64(len(q.ch)))
		}
	}

}

func writer(q *applicationQueue, writerID int, metric *metrics,
	tracer trace.Tracer, jaegerEnabled, ignoreSqsAttributeLimit bool) {

	debug := *q.conf.Debug
	queueID := q.conf.ID

	me := fmt.Sprintf("writer %s[%d/%d]", queueID, writerID, q.conf.Writers)

	carrierSQS := otelsqs.NewCarrier()
	carrierSNS := otelsns.NewCarrier()

	for {
		if debug {
			log.Printf("%s: ready: %s", me, q.conf.TopicArn)
		}

		//
		// read message from channel
		//

		sqsMsg := <-q.ch
		metric.gaugeBuffer(queueID, float64(len(q.ch)))
		//m := sqsMsg.sqs

		handleMessage(me, q, sqsMsg, metric, tracer, carrierSQS, carrierSNS,
			jaegerEnabled, ignoreSqsAttributeLimit)
	}

}

func handleMessage(me string, q *applicationQueue, sqsMsg message,
	metric *metrics, tracer trace.Tracer,
	carrierSQS *otelsqs.SqsCarrierAttributes,
	carrierSNS *otelsns.SnsCarrierAttributes,
	jaegerEnabled, ignoreSqsAttributeLimit bool) {

	debug := *q.conf.Debug
	queueID := q.conf.ID
	m := &sqsMsg.sqs

	//
	// Retrieve trace context from SQS message attributes
	//

	ctx := carrierSQS.Extract(context.TODO(), m.MessageAttributes)

	ctxNew, span := tracer.Start(ctx, "handleMessage")
	defer span.End()

	if debug {
		log.Printf("%s: MessageId: %s: traceID:%v", me, *m.MessageId, span.SpanContext().TraceID().String())
		log.Printf("%s: MessageId: %s: Attributes:%v", me, *m.MessageId, toJSON(m.Attributes))
		log.Printf("%s: MessageId: %s: MessageAttributes: count=%d: %v",
			me, aws.ToString(m.MessageId), len(m.MessageAttributes), toJSON(m.MessageAttributes))
		log.Printf("%s: MessageId: %s: Body:%v", me, *m.MessageId, *m.Body)
	}

	//
	// publish message to SNS topic
	//

	if published := snsPublish(ctxNew, me, q, sqsMsg, metric, tracer,
		carrierSNS, jaegerEnabled, ignoreSqsAttributeLimit); !published {
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

func snsPublish(ctx context.Context, me string, q *applicationQueue, sqsMsg message,
	metric *metrics, tracer trace.Tracer, carrierSNS *otelsns.SnsCarrierAttributes,
	jaegerEnabled, ignoreSqsAttributeLimit bool) bool {

	ctxNew, span := tracer.Start(ctx, "snsPublish")
	defer span.End()

	debug := *q.conf.Debug
	copyAttributes := *q.conf.CopyAttributes
	queueID := q.conf.ID
	m := &sqsMsg.sqs

	input := &sns.PublishInput{
		Message:           m.Body,
		TopicArn:          aws.String(q.conf.TopicArn),
		MessageAttributes: make(map[string]sns_types.MessageAttributeValue),
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

	if jaegerEnabled && (ignoreSqsAttributeLimit || len(m.MessageAttributes) < 10) {
		//
		// Inject trace context into SNS message attributes
		//
		carrierSNS.Inject(ctxNew, input.MessageAttributes)
	}

	if debug {
		log.Printf("%s: MessageId: %s: MessageAttributes: count=%d: %v",
			me, aws.ToString(m.MessageId), len(input.MessageAttributes),
			toJSON(input.MessageAttributes))
	}

	result, errPub := q.sns.Publish(context.TODO(), input)
	if errPub != nil {
		log.Printf("%s: sns.Publish: error: %v, sleeping %v",
			me, errPub, q.conf.ErrorCooldownWrite)
		metric.incPublishError(queueID)
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
		metric.incDeleteError(queueID)
		time.Sleep(q.conf.ErrorCooldownDelete)
		q.putStatus(errDelete)
		return false
	}

	return true
}
