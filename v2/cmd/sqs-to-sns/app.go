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
			app.startReader(q, root)
		}()
		go func() {
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

	q.readers.Add(1)
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
					go func() {
						const siblingIsRoot = false // spawned sibling is never root
						app.startReader(q, siblingIsRoot)
					}()
				}
			}
		} else {
			// we are non-root, we might exit. spawned siblings never exit.
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

func (app *application) startPublisher(q *queue, root bool) {
	const me = "publisher"
	slog.Info(me, "root", root)

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
		// TODO
		// now we have no messages on our hands.
		// we can scale up or down:
		// root: might scale up by spawning sibling.
		// non-root: might scale down by exiting.
		//
	}
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
	queueCfg  queueConfig
	forwardCh chan message
	deleteCh  chan message
	readers   atomic.Int64
	publish   *pool
}

type message struct {
	sqsMessage *sqstypes.Message
	received   time.Time
}
