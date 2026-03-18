// Package main implements the tool.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/udhos/boilerplate/envconfig"
	"gopkg.in/yaml.v3"
)

func main() {

	me := filepath.Base(os.Args[0])

	cfg := newConfig(me)

	//app := newApp(cfg, receiveReal)
	app := newApp(cfg, receiveMock)

	app.run()

	select {} // FIXME WRITEME TODO XXX graceful shutdown: stop readers, etc
}

func receiveReal(q *queue) (message, error) {
	time.Sleep(500 * time.Millisecond)
	return message{}, fmt.Errorf("receiveReal: WRITEME: %v", q)
}

func receiveMock(_ *queue) (message, error) {
	time.Sleep(2 * time.Second)
	body := "test"
	id := time.Now().Format(time.RFC3339)
	return message{
		sqsMessages: []sqstypes.Message{
			{
				Body:      aws.String(body),
				MessageId: aws.String(id),
			},
		},
	}, nil
}

func newApp(cfg config, receive receiveFunc) *application {
	app := &application{
		cfg:     cfg,
		receive: receive,
	}

	for _, queueCfg := range cfg.queues {
		q := &queue{
			queueCfg:  queueCfg,
			forwardCh: make(chan message, 100),
			deleteCh:  make(chan message, 100),
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

func (app *application) startReader(q *queue, root bool) {
	const me = "reader"
	slog.Info(me, "root", root)
	for {
		msg, err := app.receive(q)
		if err != nil {
			const cooldown = time.Second
			slog.Error(me,
				"error", err,
				"sleeping", cooldown)
			time.Sleep(cooldown)
			continue
		}

		for _, m := range msg.sqsMessages {
			slog.Info(me, "message", aws.ToString(m.MessageId))
		}

		q.forwardCh <- msg
	}
}

func (app *application) startPublisher(q *queue, root bool) {
	const me = "publisher"
	slog.Info(me, "root", root)
	for {
		msg := <-q.forwardCh

		for _, m := range msg.sqsMessages {
			slog.Info(me, "message", aws.ToString(m.MessageId))
		}

		q.deleteCh <- msg
	}
}

func (app *application) startJanitor(q *queue, root bool) {
	const me = "janitor"
	slog.Info(me, "root", root)
	for {
		msg := <-q.deleteCh

		for _, m := range msg.sqsMessages {
			slog.Info(me, "message", aws.ToString(m.MessageId))
		}
	}
}

type receiveFunc func(q *queue) (message, error)

type application struct {
	receive receiveFunc
	cfg     config
	queues  []*queue
}

type queue struct {
	queueCfg  queueConfig
	forwardCh chan message
	deleteCh  chan message
}

type message struct {
	sqsMessages []sqstypes.Message
}

type config struct {
	queueListFile string
	queues        []queueConfig
}

type queueConfig struct {
	QueueURL     string `yaml:"queue_url"`
	QueueRoleArn string `yaml:"queue_role_arn"`
	TopicArn     string `yaml:"topic_arn"`
	TopicRoleArn string `yaml:"topic_role_arn"`
}

func newConfig(sessionName string) config {

	env := envconfig.NewSimple(sessionName)

	cfg := config{
		queueListFile: env.String("QUEUES", "queues.yaml"),
	}

	cfg.queues = loadQueueConf(cfg)

	return cfg
}

func loadQueueConf(cfg config) []queueConfig {
	queuesFile := cfg.queueListFile
	const me = "loadQueueConf"
	var queues []queueConfig
	buf, errRead := os.ReadFile(queuesFile)
	if errRead != nil {
		fatalf("%s: load queues: %s: %v",
			me, queuesFile, errRead)
	}
	errYaml := yaml.Unmarshal(buf, &queues)
	if errYaml != nil {
		fatalf("%s: parse yaml: %s: %v",
			me, queuesFile, errYaml)
	}
	return queues
}

func fatalf(format string, v ...any) {
	slog.Error("FATAL: " + fmt.Sprintf(format, v...))
	os.Exit(1)
}
