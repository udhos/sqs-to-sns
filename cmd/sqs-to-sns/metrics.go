package main

import (
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/udhos/dogstatsdclient/dogstatsdclient"
)

type prom struct {
	registry *prometheus.Registry
}

func (p *prom) handler() http.Handler {
	return promhttp.InstrumentMetricHandler(
		p.registry, promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{}),
	)
}

func newProm() *prom {
	registry := prometheus.NewRegistry()
	return &prom{
		registry: registry,
	}
}

func serveMetrics(p *prom, addr, path string) {
	const me = "serveMetrics"
	log.Printf("%s: starting metrics server at: %s %s", me, addr, path)
	http.Handle(path, p.handler())
	err := http.ListenAndServe(addr, nil)
	log.Fatalf("%s: ListenAndServe error: %v", me, err)
}

type metrics struct {
	promEnable      bool
	dogstatsdClient *dogstatsdclient.Client

	buffer *prometheus.GaugeVec

	receive         *prometheus.CounterVec
	receiveError    *prometheus.CounterVec
	receiveEmpty    *prometheus.CounterVec
	receiveMessages *prometheus.CounterVec

	publishError *prometheus.CounterVec

	deleteError *prometheus.CounterVec

	delivery *prometheus.CounterVec
	latency  *prometheus.HistogramVec
}

const (
	countSuffix = "_total"

	bufferName = "buffer"

	receiveName         = "receive" + countSuffix
	receiveErrorName    = "receive_error" + countSuffix
	receiveEmptyName    = "receive_empty" + countSuffix
	receiveMessagesName = "receive_messages" + countSuffix

	publishErrorName = "publish_error" + countSuffix

	deleteErrorName = "delete_error" + countSuffix

	deliveryName = "delivery" + countSuffix
	latencyName  = "delivery_duration_seconds"
)

func newCounter(p *prom, namespace, name, desc string) *prometheus.CounterVec {
	const me = "newCounter"

	c := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      name,
			Help:      desc,
		},
		[]string{"queue"},
	)

	if err := p.registry.Register(c); err != nil {
		log.Fatalf("%s: receive was not registered: %s", me, err)
	}

	return c
}

func newMetrics(p *prom, dogstatsdEnable, dogstatsdDebug bool,
	namespace string, latencyBuckets []float64) *metrics {
	const me = "newMetrics"

	m := &metrics{
		promEnable: p != nil,
	}

	if m.promEnable {

		//
		// buffer
		//

		buffer := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      bufferName,
				Help:      "How many SQS messages are buffered with us, partitioned by queue.",
			},
			[]string{"queue"},
		)

		if err := p.registry.Register(buffer); err != nil {
			log.Fatalf("%s: buffer was not registered: %s", me, err)
		}

		//
		// latency
		//

		latency := prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      latencyName,
				Help:      "How long it took to fully process the delivery, partitioned by queue.",
				Buckets:   latencyBuckets,
			},
			[]string{"queue"},
		)

		if err := p.registry.Register(latency); err != nil {
			log.Fatalf("%s: latency was not registered: %s", me, err)
		}

		//
		// all metrics
		//

		m.buffer = buffer
		m.receive = newCounter(p, namespace, receiveName, "How many SQS receives called, partitioned by queue.")
		m.receiveError = newCounter(p, namespace, receiveErrorName, "How many SQS receives errored, partitioned by queue.")
		m.receiveEmpty = newCounter(p, namespace, receiveEmptyName, "How many SQS empty receives, partitioned by queue.")
		m.receiveMessages = newCounter(p, namespace, receiveMessagesName, "How many SQS messages received, partitioned by queue.")
		m.publishError = newCounter(p, namespace, publishErrorName, "How many SNS publishes errored, partitioned by queue.")
		m.deleteError = newCounter(p, namespace, deleteErrorName, "How many SQS deletes errored, partitioned by queue.")
		m.delivery = newCounter(p, namespace, deliveryName, "How many SQS deliveries fully processed, partitioned by queue.")
		m.latency = latency
	}

	if dogstatsdEnable {
		//
		// dogstatsd
		//

		options := dogstatsdclient.Options{
			Namespace: namespace,
			Debug:     dogstatsdDebug,
		}

		client, errClient := dogstatsdclient.New(options)
		if errClient != nil {
			log.Fatalf("%s: dogstatsd client error: %s", me, errClient)
		}

		m.dogstatsdClient = client
	}

	return m
}

func (m *metrics) recordDelivery(queue string, latency time.Duration) {
	if m.promEnable {
		m.delivery.WithLabelValues(queue).Inc()
		m.latency.WithLabelValues(queue).Observe(latency.Seconds())
	}
	if m.dogstatsdClient != nil {
		tags := []string{"queue:" + queue}
		m.dogstatsdClient.Count("delivery", 1, tags, 1)
		m.dogstatsdClient.TimeInMilliseconds("delivery_duration_milliseconds",
			float64(latency.Milliseconds()), tags, 1)
	}
}

func (m *metrics) gaugeBuffer(queue string, value float64) {
	if m.promEnable {
		m.buffer.WithLabelValues(queue).Set(value)
	}
	if m.dogstatsdClient != nil {
		tags := []string{"queue:" + queue}
		m.dogstatsdClient.Gauge("buffer", value, tags, 1)
	}
}

func (m *metrics) incReceive(queue string) {
	if m.promEnable {
		m.receive.WithLabelValues(queue).Inc()
	}
	if m.dogstatsdClient != nil {
		tags := []string{"queue:" + queue}
		m.dogstatsdClient.Count("receive", 1, tags, 1)
	}
}

func (m *metrics) incReceiveError(queue string) {
	if m.promEnable {
		m.receiveError.WithLabelValues(queue).Inc()
	}
	if m.dogstatsdClient != nil {
		tags := []string{"queue:" + queue}
		m.dogstatsdClient.Count("receive_error", 1, tags, 1)
	}
}

func (m *metrics) incReceiveEmpty(queue string) {
	if m.promEnable {
		m.receiveEmpty.WithLabelValues(queue).Inc()
	}
	if m.dogstatsdClient != nil {
		tags := []string{"queue:" + queue}
		m.dogstatsdClient.Count("receive_empty", 1, tags, 1)
	}
}

func (m *metrics) incReceiveMessages(queue string, value float64) {
	if m.promEnable {
		m.receiveMessages.WithLabelValues(queue).Add(value)
	}
	if m.dogstatsdClient != nil {
		tags := []string{"queue:" + queue}
		m.dogstatsdClient.Count("receive_messages", int64(value), tags, 1)
	}
}

func (m *metrics) incPublishError(queue string) {
	if m.promEnable {
		m.publishError.WithLabelValues(queue).Inc()
	}
	if m.dogstatsdClient != nil {
		tags := []string{"queue:" + queue}
		m.dogstatsdClient.Count("publish_error", 1, tags, 1)
	}
}

func (m *metrics) incDeleteError(queue string) {
	if m.promEnable {
		m.deleteError.WithLabelValues(queue).Inc()
	}
	if m.dogstatsdClient != nil {
		tags := []string{"queue:" + queue}
		m.dogstatsdClient.Count("delete_error", 1, tags, 1)
	}
}
