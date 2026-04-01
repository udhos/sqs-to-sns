package main

import (
	"math"
	"time"

	"github.com/udhos/dogstatsdclient/dogstatsdclient"
)

func exportDogstatsd(namespace string, dogstatsdInterval time.Duration,
	sampleRate float64, queues []*queue) error {

	c, errClient := dogstatsdclient.New(dogstatsdclient.Options{
		Namespace: namespace,
	})

	if errClient != nil {
		return errClient
	}

	go func() {
		ticker := time.NewTicker(dogstatsdInterval) // 20s interval
		for range ticker.C {
			for _, q := range queues {
				// We capture gauge metrics here because our gauges
				// are smart enough to keep min/avg/max for the full interval.

				// Record current goroutine state before harvesting
				q.stats.receiverGoroutines.record(uint64(q.readers.Load()))
				q.stats.publisherGoroutines.record(uint64(q.publishers.Load()))
				q.stats.janitorGoroutines.record(uint64(q.janitors.Load()))

				// Also record channel load even if idle
				q.stats.publishChLoad.record(uint64(channelLoad(q.publishCh) * 100))
				q.stats.deleteChLoad.record(uint64(channelLoad(q.deleteCh) * 100))

				tags := []string{"queue_id:" + q.queueCfg.ID}
				snap := q.stats.harvest()
				c.Count("receive_errors", int64(snap.receiveErrors), tags, sampleRate)
				c.Count("publish_errors", int64(snap.publishErrors), tags, sampleRate)
				c.Count("delete_errors", int64(snap.deleteErrors), tags, sampleRate)
				c.Count("receives", int64(snap.receives), tags, sampleRate)
				c.Count("publishes", int64(snap.publishes), tags, sampleRate)
				c.Count("deletes", int64(snap.deletes), tags, sampleRate)
				c.Count("partial_publishes", int64(snap.partialPublishes), tags, sampleRate)
				c.Count("partial_deletes", int64(snap.partialDeletes), tags, sampleRate)
				c.Count("dropped_messages", int64(snap.droppedMessages), tags, sampleRate)
				c.Count("received_messages", int64(snap.receivedMessages), tags, sampleRate)
				c.Count("published_messages", int64(snap.publishedMessages), tags, sampleRate)
				c.Count("deleted_messages", int64(snap.deletedMessages), tags, sampleRate)
				c.Count("goroutine_spawns", int64(snap.goroutineSpawns), tags, sampleRate)
				c.Count("goroutine_exits", int64(snap.goroutineExits), tags, sampleRate)
				dogstatsdGauge(c, "publish_channel_load", snap.publishChLoad, tags, sampleRate)
				dogstatsdGauge(c, "delete_channel_load", snap.deleteChLoad, tags, sampleRate)
				dogstatsdGauge(c, "forward_latency", snap.forwardLatency, tags, sampleRate)
				dogstatsdGauge(c, "receiver_goroutines", snap.receiverGoroutines, tags, sampleRate)
				dogstatsdGauge(c, "publisher_goroutines", snap.publisherGoroutines, tags, sampleRate)
				dogstatsdGauge(c, "janitor_goroutines", snap.janitorGoroutines, tags, sampleRate)
			}
		}
	}()

	return nil
}

func dogstatsdGauge(c *dogstatsdclient.Client, name string, value gaugeSnapshot,
	tags []string, sampleRate float64) {

	// If min is still the sentinel, it means record() was never called this interval.
	if value.min == math.MaxUint64 {
		return
	}

	c.Gauge(name+"_min", float64(value.min), tags, sampleRate)
	c.Gauge(name+"_avg", value.avg, tags, sampleRate)
	c.Gauge(name+"_max", float64(value.max), tags, sampleRate)
}
