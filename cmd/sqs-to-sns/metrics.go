package main

import (
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type prom struct {
	registerer prometheus.Registerer
	gatherer   prometheus.Gatherer
}

func (p *prom) handler() http.Handler {
	return promhttp.InstrumentMetricHandler(
		p.registerer, promhttp.HandlerFor(p.gatherer, promhttp.HandlerOpts{}),
	)
}

func newProm() *prom {
	registry := prometheus.NewRegistry()
	return &prom{
		registerer: registry,
		gatherer:   registry,
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

	if err := p.registerer.Register(c); err != nil {
		log.Fatalf("%s: receive was not registered: %s", me, err)
	}

	return c
}

func newMetrics(p *prom, namespace string, latencyBuckets []float64) *metrics {
	const me = "newMetrics"

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

	if err := p.registerer.Register(buffer); err != nil {
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

	if err := p.registerer.Register(latency); err != nil {
		log.Fatalf("%s: latency was not registered: %s", me, err)
	}

	//
	// all metrics
	//

	m := &metrics{
		buffer:          buffer,
		receive:         newCounter(p, namespace, receiveName, "How many SQS receives called, partitioned by queue."),
		receiveError:    newCounter(p, namespace, receiveErrorName, "How many SQS receives errored, partitioned by queue."),
		receiveEmpty:    newCounter(p, namespace, receiveEmptyName, "How many SQS empty receives, partitioned by queue."),
		receiveMessages: newCounter(p, namespace, receiveMessagesName, "How many SQS messages received, partitioned by queue."),
		publishError:    newCounter(p, namespace, publishErrorName, "How many SNS publishes errored, partitioned by queue."),
		deleteError:     newCounter(p, namespace, deleteErrorName, "How many SQS deletes errored, partitioned by queue."),
		delivery:        newCounter(p, namespace, deliveryName, "How many SQS deliveries fully processed, partitioned by queue."),
		latency:         latency,
	}

	return m
}

func (m *metrics) recordDelivery(queue string, latency time.Duration) {
	m.delivery.WithLabelValues(queue).Inc()
	m.latency.WithLabelValues(queue).Observe(float64(latency) / float64(time.Second))
}
