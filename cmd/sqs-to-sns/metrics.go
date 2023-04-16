package main

import (
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func serveMetrics(addr, path string) {
	const me = "serveMetrics"
	log.Printf("%s: starting metrics server at: %s %s", me, addr, path)
	http.Handle(path, promhttp.Handler())
	err := http.ListenAndServe(addr, nil)
	log.Printf("%s: ListenAndServe error: %v", me, err)
}

type metrics struct {
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
	countSuffix = "_count" // or _total ??

	receiveName         = "receive" + countSuffix
	receiveErrorName    = "receive_error" + countSuffix
	receiveEmptyName    = "receive_empty" + countSuffix
	receiveMessagesName = "receive_messages" + countSuffix

	publishErrorName = "publish_error" + countSuffix

	deleteErrorName = "delete_error" + countSuffix

	deliveryName = "delivery" + countSuffix
	latencyName  = "delivery_duration_seconds"
)

func newCounter(namespace, name, desc string) *prometheus.CounterVec {
	const me = "newCounter"

	c := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      name,
			Help:      desc,
		},
		[]string{"queue"},
	)

	if err := prometheus.Register(c); err != nil {
		log.Fatalf("%s: receive was not registered: %s", me, err)
	}

	return c
}

func newMetrics(namespace string) *metrics {
	const me = "newMetrics"

	m := &metrics{}

	m.receive = newCounter(namespace, receiveName, "How many SQS receives called, partitioned by queue.")
	m.receiveError = newCounter(namespace, receiveErrorName, "How many SQS receives errored, partitioned by queue.")
	m.receiveEmpty = newCounter(namespace, receiveEmptyName, "How many SQS empty receives, partitioned by queue.")
	m.receiveMessages = newCounter(namespace, receiveMessagesName, "How many SQS messages received, partitioned by queue.")
	m.publishError = newCounter(namespace, publishErrorName, "How many SNS publishes errored, partitioned by queue.")
	m.deleteError = newCounter(namespace, deleteErrorName, "How many SQS deletes errored, partitioned by queue.")

	//
	// delivery
	//

	m.delivery = newCounter(namespace, deliveryName, "How many SQS deliveries fully processed, partitioned by queue.")

	//
	// latency
	//

	m.latency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      latencyName,
		Help:      "How long it took to fully process the delivery, partitioned by queue.",
		Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
	},
		[]string{"queue"},
	)

	if err := prometheus.Register(m.latency); err != nil {
		log.Fatalf("%s: latency was not registered: %s", me, err)
	}

	return m
}

func (m *metrics) recordDelivery(queue string, latency time.Duration) {
	m.delivery.WithLabelValues(queue).Inc()
	m.latency.WithLabelValues(queue).Observe(float64(latency) / float64(time.Second))
}