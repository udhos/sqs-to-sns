package main

import (
	"math"
	"sync/atomic"
)

// stats are recorded per-queue.
type stats struct {
	receiveErrors atomic.Uint64 // count
	publishErrors atomic.Uint64 // count
	deleteErrors  atomic.Uint64 // count

	publishChLoad  gauge // percentage 0..100 (100 * len/cap)
	deleteChLoad   gauge // percentage 0..100 (100 * len/cap)
	forwardLatency gauge // milliseconds
}

type statsSnapshot struct {
	receiveErrors uint64 // count
	publishErrors uint64 // count
	deleteErrors  uint64 // count

	publishChLoad  gaugeSnapshot // percentage 0..100 (100 * len/cap)
	deleteChLoad   gaugeSnapshot // percentage 0..100 (100 * len/cap)
	forwardLatency gaugeSnapshot // milliseconds
}

func initStats(s *stats) {
	s.publishChLoad.min.Store(math.MaxUint64)
	s.deleteChLoad.min.Store(math.MaxUint64)
	s.forwardLatency.min.Store(math.MaxUint64)
}

func (s *stats) harvest() statsSnapshot {
	return statsSnapshot{
		// Use Swap(0) to get the delta (count since last harvest)
		receiveErrors: s.receiveErrors.Swap(0),
		publishErrors: s.publishErrors.Swap(0),
		deleteErrors:  s.deleteErrors.Swap(0),

		// Gauges already use Swap(0) internally
		publishChLoad:  s.publishChLoad.harvest(),
		deleteChLoad:   s.deleteChLoad.harvest(),
		forwardLatency: s.forwardLatency.harvest(),
	}
}

// gauges uses count/sum to calcutage avg.
type gauge struct {
	sum   atomic.Uint64
	count atomic.Uint64
	min   atomic.Uint64
	max   atomic.Uint64
}

type gaugeSnapshot struct {
	min uint64
	max uint64
	avg float64
}

func (g *gauge) record(val uint64) {
	g.sum.Add(val)
	g.count.Add(1)

	// Update Max
	for {
		old := g.max.Load()
		if val <= old {
			break
		}
		if g.max.CompareAndSwap(old, val) {
			break
		}
	}

	// Update Min
	for {
		old := g.min.Load()
		if val >= old {
			break
		}
		if g.min.CompareAndSwap(old, val) {
			break
		}
	}
}

func (g *gauge) harvest() gaugeSnapshot {
	s := g.sum.Swap(0)
	c := g.count.Swap(0)
	mn := g.min.Swap(math.MaxUint64)
	mx := g.max.Swap(0)

	var avg float64

	if c > 0 {
		avg = float64(s) / float64(c)
	}

	return gaugeSnapshot{min: mn, max: mx, avg: avg}
}
