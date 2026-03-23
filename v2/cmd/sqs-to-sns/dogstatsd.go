package main

import (
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
				tags := []string{"queue_id:" + q.queueCfg.ID}
				snap := q.stats.harvest()
				c.Count("receive_errors", int64(snap.receiveErrors), tags, sampleRate)
				c.Count("publish_errors", int64(snap.publishErrors), tags, sampleRate)
				c.Count("delete_errors", int64(snap.deleteErrors), tags, sampleRate)
				dogstatsdGauge(c, "publish_channel_load", snap.publishChLoad, tags, sampleRate)
				dogstatsdGauge(c, "delete_channel_load", snap.deleteChLoad, tags, sampleRate)
				dogstatsdGauge(c, "forward_latency", snap.forwardLatency, tags, sampleRate)
			}
		}
	}()

	return nil
}

func dogstatsdGauge(c *dogstatsdclient.Client, name string, value gaugeSnapshot,
	tags []string, sampleRate float64) {
	// If max is 0, no events occurred. Skip to avoid sending math.MaxUint64 min.
	if value.max == 0 {
		return
	}
	c.Gauge(name+"_min", float64(value.min), tags, sampleRate)
	c.Gauge(name+"_avg", value.avg, tags, sampleRate)
	c.Gauge(name+"_max", float64(value.max), tags, sampleRate)
}
