package main

import (
	"log/slog"
	"sync/atomic"
	"time"
)

func newApp(cfg config, receive receiver, publish publisher,
	deleter deleter) *application {

	app := &application{
		cfg:     cfg,
		receive: receive,
		publish: publish,
		delete:  deleter,
	}

	for _, queueCfg := range cfg.queues {
		q := &queue{
			queueCfg:    queueCfg,
			publishCh:   make(chan message, queueCfg.BufferSizePublish),
			deleteCh:    make(chan message, queueCfg.BufferSizeDelete),
			publishPool: newPool(maxSnsPublishPayload),

			// FIXME
			// delete does not care about payload size.
			// create a specialized version of pool that does not look at payload size.
			// for now we set an unreachable limit.
			deletePool: newPool(100 * maxSnsPublishPayload),
		}
		app.queues = append(app.queues, q)
	}

	return app
}

func (app *application) run() {
	for _, q := range app.queues {
		const root = true
		go func() {
			q.readers.Add(1)
			app.startReader(q, root)
		}()
		go func() {
			q.publishers.Add(1)
			app.startPublisher(q, root)
		}()
		go func() {
			q.janitors.Add(1)
			app.startJanitor(q, root)
		}()
	}
}

func (app *application) stopReaders() {
	const me = "stopReaders"
	infof("stopping readers")
	for _, q := range app.queues {
		if err := app.receive.stop(q); err != nil {
			slog.Error(me,
				"queue_id", q.queueCfg.ID,
				"error", err)
		}
	}
}

func (app *application) startReader(q *queue, root bool) {
	const me = "reader"

	defer q.readers.Add(-1)

	for {
		msg, mustStop, err := app.receive.receive(q)
		if err != nil {

			if mustStop {
				// error but stopped, log and exit.
				slog.Error(me, "error", err)
				break
			}

			const errorCooldown = time.Second
			slog.Error(me, "error", err, "sleeping", errorCooldown)
			time.Sleep(errorCooldown)
			continue
		}

		emptyReceive := len(msg) == 0

		for _, m := range msg {
			q.publishCh <- m
		}

		if mustStop {
			// exit right after forwarding all messages.
			// both root and non-root must exit because
			// we are shutting down and should no longer
			// receive anything.
			break
		}

		//
		// now we have no messages on our hands.
		// we can scale up or down:
		// root: might scale up by spawning sibling.
		// non-root: might scale down by exiting.
		//
		if root {
			// we are root, we might spawn sibling.
			if len(msg) >= 10 {
				// we handled a full batch, then we should spawn a sibling.
				//
				// we are the unique (root) goroutine spawning siblings,
				// so it is enough to check we are under the limit.
				if q.readers.Load() < int64(q.queueCfg.LimitReaders) {
					q.readers.Add(1)
					go func() {
						const siblingIsRoot = false // spawned sibling is never root
						app.startReader(q, siblingIsRoot)
					}()
				}
			}
			if emptyReceive {
				// we are root.
				// we might be alone hitting the API with empty receives.
				// do not hammer the API.
				// this might be excessive since we use long polling.
				const emptyReceiveCooldown = time.Second
				time.Sleep(emptyReceiveCooldown)
			}
			continue
		}

		// we are non-root, we might exit. root never exits.
		if emptyReceive {
			// we handled no message, then exit.
			break
		}
	}
}

const (
	watermarkLow  = 1.0 / 3.0
	watermarkHigh = 2.0 / 3.0
)

func (app *application) startPublisher(q *queue, root bool) {

	defer q.publishers.Add(-1)

	if root {
		// Spawn global periodic flusher.
		// This is the ONLY place where a time-based flush happens.
		// It ensures we eventually flush partial batches.
		go func() {
			ticker := time.NewTicker(app.cfg.flushIntervalPublish)
			for range ticker.C {
				m := q.publishPool.getAvailable()
				if len(m) > 0 {
					app.batchPublish(q, m)
				}
			}
		}()
	}

	for msg := range q.publishCh {
		q.publishPool.add(msg)

		// drain full batches.
		for {
			// attempt to get full 10-message batch
			m, found := q.publishPool.getFullBatch()
			if !found {
				break // no full batch
			}
			// got a full batch
			app.batchPublish(q, m)
		}

		//
		// we can scale up or down:
		// root: might scale up by spawning sibling.
		// non-root: might scale down by exiting.
		//
		load := channelLoad(q.publishCh)
		if root {
			// we are root, we might spawn sibling.
			if load > watermarkHigh {
				// incoming channel is getting full, then we should spawn a sibling.
				//
				// we are the unique (root) goroutine spawning siblings,
				// so it is enough to check we are under the limit.
				if q.publishers.Load() < int64(q.queueCfg.LimitPublishers) {
					q.publishers.Add(1)
					go func() {
						const siblingIsRoot = false // spawned sibling is never root
						app.startPublisher(q, siblingIsRoot)
					}()
				}
			}
			continue
		}

		// we are non-root, we might exit. root never exits.
		if load < watermarkLow {
			// incoming channel is getting empty, then exit.
			break
		}

	} // for
}

func channelLoad(ch chan message) float32 {
	return float32(len(ch)) / float32(cap(ch))
}

func (app *application) batchPublish(q *queue, msg []message) {

	const me = "batchPublish"

	infof("%s: %d", me, len(msg))

	pub, errPub := app.publish.publish(q, msg)
	if errPub != nil {
		slog.Error(me,
			"queue_id", q.queueCfg.ID,
			"error", q.queueCfg.ID)
		return
	}

	for _, m := range pub {
		q.deleteCh <- m
	}
}

func (app *application) batchDelete(q *queue, msg []message) {

	const me = "batchDelete"

	infof("%s: %d", me, len(msg))

	errDel := app.delete.delete(q, msg)
	if errDel != nil {
		slog.Error(me,
			"queue_id", q.queueCfg.ID,
			"error", q.queueCfg.ID)
		return
	}
}

func (app *application) startJanitor(q *queue, root bool) {

	defer q.janitors.Add(-1)

	if root {
		// Spawn global periodic flusher.
		// This is the ONLY place where a time-based flush happens.
		// It ensures we eventually flush partial batches.
		go func() {
			ticker := time.NewTicker(app.cfg.flushIntervalDelete)
			for range ticker.C {
				m := q.deletePool.getAvailable()
				if len(m) > 0 {
					app.batchDelete(q, m)
				}
			}
		}()
	}

	for msg := range q.deleteCh {
		q.deletePool.add(msg)

		// drain full batches.
		for {
			// attempt to get full 10-message batch
			m, found := q.deletePool.getFullBatch()
			if !found {
				break // no full batch
			}
			// got a full batch
			app.batchDelete(q, m)
		}

		//
		// we can scale up or down:
		// root: might scale up by spawning sibling.
		// non-root: might scale down by exiting.
		//
		load := channelLoad(q.deleteCh)
		if root {
			// we are root, we might spawn sibling.
			if load > watermarkHigh {
				// incoming channel is getting full, then we should spawn a sibling.
				//
				// we are the unique (root) goroutine spawning siblings,
				// so it is enough to check we are under the limit.
				if q.janitors.Load() < int64(q.queueCfg.LimitDeleters) {
					q.janitors.Add(1)
					go func() {
						const siblingIsRoot = false // spawned sibling is never root
						app.startJanitor(q, siblingIsRoot)
					}()
				}
			}
			continue
		}

		// we are non-root, we might exit. root never exits.
		if load < watermarkLow {
			// incoming channel is getting empty, then exit.
			break
		}

	} // for
}

type application struct {
	receive receiver
	publish publisher
	delete  deleter
	cfg     config
	queues  []*queue
}

type receiver interface {
	receive(q *queue) ([]message, bool, error)
	stop(q *queue) error
}

type publisher interface {
	publish(q *queue, messages []message) ([]message, error)
}

type deleter interface {
	delete(q *queue, messages []message) error
}

type queue struct {
	queueCfg    queueConfig
	publishCh   chan message
	deleteCh    chan message
	readers     atomic.Int64
	publishers  atomic.Int64
	janitors    atomic.Int64
	publishPool *pool
	deletePool  *pool
}
