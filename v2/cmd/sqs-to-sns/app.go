package main

import (
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func newApp(cfg config,
	clientGenerator func(queueCfg queueConfig) (receiver, publisher, deleter)) *application {

	app := &application{
		health: newHealthServer(cfg.healthAddr, cfg.healthPath),
		cfg:    cfg,
	}

	for _, queueCfg := range cfg.queues {

		receive, publish, deleter := clientGenerator(queueCfg)

		q := &queue{
			queueCfg:    queueCfg,
			publishCh:   make(chan message, queueCfg.BufferSizePublish),
			deleteCh:    make(chan message, queueCfg.BufferSizeDelete),
			publishPool: newPoolV2(maxSnsPublishPayload), // Byte-size-limited
			deletePool:  newPoolV1(),                     // NOT byte-size-limited

			receive: receive,
			publish: publish,
			delete:  deleter,
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
	for _, q := range app.queues {
		q.receive.stop(q)
	}
}

func (app *application) startReader(q *queue, root bool) {
	const me = "reader"

	defer q.readers.Add(-1)

	for {
		msg, mustStop, err := q.receive.receive(q)
		if err != nil {

			if mustStop {
				// error but stopped, log and exit.
				slog.Error(me, "error", err)
				break
			}

			slog.Error(me,
				"queue_id", q.queueCfg.ID,
				"error", err,
				"sleeping", q.queueCfg.ReceiveErrorCooldown)
			time.Sleep(q.queueCfg.ReceiveErrorCooldown)
			continue
		}

		emptyReceive := len(msg) == 0

		for _, m := range msg {

			// debug logs - what we received
			if app.cfg.logMessageBody {
				slog.Debug(me,
					"queue_id", q.queueCfg.ID,
					"message_id", aws.ToString(m.sqsMessage.MessageId),
					"message_body", aws.ToString(m.sqsMessage.Body))
			} else {
				slog.Debug(me,
					"queue_id", q.queueCfg.ID,
					"message_id", aws.ToString(m.sqsMessage.MessageId))
			}

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

			app.health.heartbeat() // tell health check server that we are alive

			// we are root, we might spawn sibling.
			if len(msg) >= 10 {
				// we handled a full batch, then we should spawn a sibling.
				//
				// we are the unique (root) goroutine spawning siblings,
				// so it is enough to check we are under the limit.
				if q.readers.Load() < q.queueCfg.LimitReaders {
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
				// this is not critical as long as we use 20s long polling.
				// but we keep this in place (1s default) for extra stability.
				time.Sleep(q.queueCfg.EmptyReceiveCooldown)
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
				// Only partial-flush if we haven't batch-published anything in the last interval.
				last := q.lastPublishUnix.Load()
				if time.Since(time.Unix(0, last)) < app.cfg.flushIntervalPublish {
					continue
				}

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
				if q.publishers.Load() < q.queueCfg.LimitPublishers {
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

	// Record activity to keep flusher from flushing
	// partial batches without real need.
	q.lastPublishUnix.Store(time.Now().UnixNano())

	pub, errPub := q.publish.publish(q, msg)
	if errPub != nil {
		slog.Error(me,
			"queue_id", q.queueCfg.ID,
			"error", errPub,
			"sleeping", q.queueCfg.PublishErrorCooldown)
		time.Sleep(q.queueCfg.PublishErrorCooldown)
		return
	}

	for _, m := range pub {
		// debug logs - what we published
		if app.cfg.logMessageBody {
			slog.Debug(me,
				"queue_id", q.queueCfg.ID,
				"message_id", aws.ToString(m.sqsMessage.MessageId),
				"message_body", aws.ToString(m.sqsMessage.Body))
		} else {
			slog.Debug(me,
				"queue_id", q.queueCfg.ID,
				"message_id", aws.ToString(m.sqsMessage.MessageId))
		}

		q.deleteCh <- m
	}
}

func (app *application) batchDelete(q *queue, msg []message) {

	const me = "batchDelete"

	// Record activity to keep flusher from flushing
	// partial batches without real need.
	q.lastDeleteUnix.Store(time.Now().UnixNano())

	del, errDel := q.delete.delete(q, msg)
	if errDel != nil {
		slog.Error(me,
			"queue_id", q.queueCfg.ID,
			"error", errDel,
			"sleeping", q.queueCfg.DeleteErrorCooldown)
		time.Sleep(q.queueCfg.DeleteErrorCooldown)
		return
	}

	for _, m := range del {
		// debug logs - what we deleted
		if app.cfg.logMessageBody {
			slog.Debug(me,
				"queue_id", q.queueCfg.ID,
				"message_id", aws.ToString(m.sqsMessage.MessageId),
				"message_body", aws.ToString(m.sqsMessage.Body))
		} else {
			slog.Debug(me,
				"queue_id", q.queueCfg.ID,
				"message_id", aws.ToString(m.sqsMessage.MessageId))
		}
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
				// Only partial-flush if we haven't batch-deleted anything in the last interval.
				last := q.lastDeleteUnix.Load()
				if time.Since(time.Unix(0, last)) < app.cfg.flushIntervalDelete {
					continue
				}

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
				if q.janitors.Load() < q.queueCfg.LimitDeleters {
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
	health *health
	cfg    config
	queues []*queue
}

type receiver interface {
	receive(q *queue) ([]message, bool, error)
	stop(q *queue)
}

type publisher interface {
	publish(q *queue, messages []message) ([]message, error)
}

type deleter interface {
	delete(q *queue, messages []message) ([]message, error)
}

type queue struct {
	queueCfg        queueConfig
	publishCh       chan message
	deleteCh        chan message
	readers         atomic.Int64
	publishers      atomic.Int64
	janitors        atomic.Int64
	publishPool     pool
	deletePool      pool
	lastPublishUnix atomic.Int64
	lastDeleteUnix  atomic.Int64

	receive receiver
	publish publisher
	delete  deleter
}
