package main

import (
	"encoding/json"
	"os"
	"time"

	"github.com/udhos/boilerplate/envconfig"
	"gopkg.in/yaml.v3"
)

type config struct {
	queueListFile string
	exitDelay     time.Duration
	queues        []queueConfig
}

type queueConfig struct {
	ID                string `yaml:"id"`
	QueueURL          string `yaml:"queue_url"`
	QueueRoleArn      string `yaml:"queue_role_arn"`
	TopicArn          string `yaml:"topic_arn"`
	TopicRoleArn      string `yaml:"topic_role_arn"`
	BufferSizePublish int    `yaml:"buffer_size_publish"`
	BufferSizeDelete  int    `yaml:"buffer_size_delete"`
	LimitReaders      int    `yaml:"limit_readers"`
	LimitPublishers   int    `yaml:"limit_publishers"`
}

func newConfig(sessionName string) config {

	env := envconfig.NewSimple(sessionName)

	cfg := config{
		queueListFile: env.String("QUEUES", "queues.yaml"),
		exitDelay:     env.Duration("EXIT_DELAY", 5*time.Second),
	}

	cfg.queues = loadQueueConf(cfg)

	return cfg
}

const (
	defaultBufferSize       = 1000
	defaultLimitConcurrency = 50
)

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
	queues = applyQueuesDefaults(queues)
	return queues
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func applyQueuesDefaults(queues []queueConfig) []queueConfig {
	for i, q := range queues {
		queues[i] = queueDefaults(q)
		infof("queue %s: %s", q.ID, toJSON(queues[i]))
	}
	return queues
}

func queueDefaults(q queueConfig) queueConfig {
	if q.BufferSizePublish < 1 {
		q.BufferSizePublish = defaultBufferSize
	}
	if q.BufferSizeDelete < 1 {
		q.BufferSizeDelete = defaultBufferSize
	}
	if q.LimitReaders < 1 {
		q.LimitReaders = defaultLimitConcurrency
	}
	if q.LimitPublishers < 1 {
		q.LimitPublishers = defaultLimitConcurrency
	}
	return q
}
