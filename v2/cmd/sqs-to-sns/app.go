package main

import (
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func newApp(cfg config, receive receiver) *application {
	app := &application{
		cfg:     cfg,
		receive: receive,
	}

	for _, queueCfg := range cfg.queues {
		q := &queue{
			queueCfg:  queueCfg,
			forwardCh: make(chan message, queueCfg.BufferSizeForward),
			deleteCh:  make(chan message, queueCfg.BufferSizeDelete),
			publish:   newPool(),
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
			app.startJanitor(q, root)
		}()
	}
}

func (app *application) stopReaders() {
	const me = "stopReaders"
	infof("stopping readers")
	for _, q := range app.queues {
		if err := app.receive.stop(q); err != nil {
			slog.Error(me)
		}
	}
}

func (app *application) startReader(q *queue, root bool) {
	const me = "reader"
	slog.Info(me, "root", root)

	defer q.readers.Add(-1)

	for {
		msg, stopped, err := app.receive.receive(q)
		if err != nil {

			if stopped {
				// error but stopped, log and exit.
				slog.Error(me, "error", err)
				break
			}

			const cooldown = time.Second
			slog.Error(me, "error", err, "sleeping", cooldown)
			time.Sleep(cooldown)
			continue
		}

		for _, m := range msg {
			slog.Info(me, "message", aws.ToString(m.sqsMessage.MessageId))

			q.forwardCh <- m
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
				// so it is enough to check we are under the limit of readers.
				if q.readers.Load() < int64(q.queueCfg.LimitReaders) {
					q.readers.Add(1)
					go func() {
						const siblingIsRoot = false // spawned sibling is never root
						app.startReader(q, siblingIsRoot)
					}()
				}
			}
		} else {
			// we are non-root, we might exit. root never exits.
			if len(msg) == 0 {
				// we handled no message, then exit.
				break
			}
		}

		if stopped {
			// stopped, exit after forwarding message.
			break
		}
	}
}

const (
	watermarkLow  = 1.0 / 3.0
	watermarkHigh = 2.0 / 3.0
)

func (app *application) startPublisher(q *queue, root bool) {
	const me = "publisher"
	slog.Info(me, "root", root)

	defer q.publishers.Add(-1)

	const heartbeat = 500 * time.Millisecond

	timer := time.NewTimer(heartbeat)

	for {
		select {
		case msg := <-q.forwardCh:
			slog.Info(me, "message", aws.ToString(msg.sqsMessage.MessageId))
			q.publish.add(msg)

			for {
				// attempt to get full 10-message batch
				m, found := q.publish.getFullBatch()
				if !found {
					break // no full batch
				}
				// got a full batch
				batchPublish(q, m)
				timer.Reset(heartbeat) // reset after sending batch
			}
		case <-timer.C:
			// 500ms tick arrived before a full batch formed,
			// we will drain anything we have.
			m := q.publish.getAvailable()
			if len(m) > 0 {
				// got something to publish
				batchPublish(q, m)
			}
			timer.Reset(heartbeat) // reset on every tick
		}

		//
		// we can scale up or down:
		// root: might scale up by spawning sibling.
		// non-root: might scale down by exiting.
		//
		load := channelLoad(q.forwardCh)
		if root {
			// we are root, we might spawn sibling.
			if load > watermarkHigh {
				// incoming channel is getting full, then we should spawn a sibling.
				//
				// we are the unique (root) goroutine spawning siblings,
				// so it is enough to check we are under the limit of readers.
				if q.publishers.Load() < int64(q.queueCfg.LimitPublishers) {
					q.publishers.Add(1)
					go func() {
						const siblingIsRoot = false // spawned sibling is never root
						app.startPublisher(q, siblingIsRoot)
					}()
				}
			}
		} else {
			// we are non-root, we might exit. root never exits.
			if load < watermarkLow && q.publish.isEmpty() {
				// incoming channel is getting empty AND our pool is empty, then exit.
				break
			}
		}

	} // for
}

func channelLoad(ch chan message) float32 {
	return float32(len(ch)) / float32(cap(ch))
}

func batchPublish(q *queue, msg []message) {
	fatalf("call batch publisher: %v", msg)
	for _, m := range msg {
		fatalf("only published messages should be sent to deleteCh")
		q.deleteCh <- m
	}
}

func (app *application) startJanitor(q *queue, root bool) {
	const me = "janitor"
	slog.Info(me, "root", root)
	for {
		msg := <-q.deleteCh

		slog.Info(me, "message", aws.ToString(msg.sqsMessage.MessageId))
	}
}

type application struct {
	receive receiver
	cfg     config
	queues  []*queue
}

type receiver interface {
	receive(q *queue) ([]message, bool, error)
	stop(q *queue) error
}

type queue struct {
	queueCfg   queueConfig
	forwardCh  chan message
	deleteCh   chan message
	readers    atomic.Int64
	publishers atomic.Int64
	publish    *pool
}

type message struct {
	sqsMessage *sqstypes.Message
	received   time.Time
}
