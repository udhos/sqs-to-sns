package main

import (
	"encoding/json"
	"os"
	"time"

	"github.com/udhos/boilerplate/envconfig"
	"gopkg.in/yaml.v3"
)

type config struct {
	queueListFile        string
	logMessageBody       bool
	healthPath           string
	healthAddr           string
	endpointURL          string
	exitDelay            time.Duration
	flushIntervalPublish time.Duration
	flushIntervalDelete  time.Duration
	awsAPITimeout        time.Duration
	queues               []queueConfig
}

type queueConfig struct {
	ID                   string        `yaml:"id"`
	QueueURL             string        `yaml:"queue_url"`
	QueueRoleArn         string        `yaml:"queue_role_arn"`
	TopicArn             string        `yaml:"topic_arn"`
	TopicRoleArn         string        `yaml:"topic_role_arn"`
	BufferSizePublish    int           `yaml:"buffer_size_publish"`
	BufferSizeDelete     int           `yaml:"buffer_size_delete"`
	LimitReaders         int64         `yaml:"limit_readers"`
	LimitPublishers      int64         `yaml:"limit_publishers"`
	LimitDeleters        int64         `yaml:"limit_deleters"`
	MaxNumberOfMessages  int32         `yaml:"max_number_of_messages"` // 1..10 (default 10)
	WaitTimeSeconds      *int32        `yaml:"wait_time_seconds"`      // 0..20 (default 20)
	CopyAttributes       *bool         `yaml:"copy_attributes"`
	CopyMesssageGroupID  *bool         `yaml:"copy_message_group_id"`
	EmptyReceiveCooldown time.Duration `yaml:"empty_receive_cooldown"`
	ReceiveErrorCooldown time.Duration `yaml:"receive_error_cooldown"`
	PublishErrorCooldown time.Duration `yaml:"publish_error_cooldown"`
	DeleteErrorCooldown  time.Duration `yaml:"delete_error_cooldown"`
}

func newConfig(env *envconfig.Env) config {

	cfg := config{
		queueListFile:        env.String("QUEUES", "queues.yaml"),
		logMessageBody:       env.Bool("LOG_MESSAGE_BODY", false),
		healthPath:           env.String("HEALTH_PATH", "/health"),
		healthAddr:           env.String("HEALTH_ADDR", ":8080"),
		endpointURL:          env.String("ENDPOINT_URL", ""),
		exitDelay:            env.Duration("EXIT_DELAY", 5*time.Second),
		flushIntervalPublish: env.Duration("FLUSH_INTERVAL_PUBLISH", 500*time.Millisecond),
		flushIntervalDelete:  env.Duration("FLUSH_INTERVAL_DELETE", time.Second),
		awsAPITimeout:        env.Duration("AWS_API_TIMEOUT", 30*time.Second),
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

const (
	defaultBufferSize                 = 1000
	defaultLimitConcurrency           = 50
	defaultNumberOfMessages           = 10
	defaultWaitTimeSeconds      int32 = 20
	defaultCopyAttributes             = true
	defaultCopyMesssageGroupID        = true
	defaultEmptyReceiveCooldown       = 1 * time.Second
	defaultReceiveErrorCooldown       = 1 * time.Second
	defaultPublishErrorCooldown       = 1 * time.Second
	defaultDeleteErrorCooldown        = 1 * time.Second
)

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
	if q.LimitDeleters < 1 {
		q.LimitDeleters = defaultLimitConcurrency
	}
	if q.MaxNumberOfMessages < 1 {
		q.MaxNumberOfMessages = defaultNumberOfMessages
	}
	if q.WaitTimeSeconds == nil {
		b := defaultWaitTimeSeconds
		q.WaitTimeSeconds = &b
	}
	if q.CopyAttributes == nil {
		b := defaultCopyAttributes
		q.CopyAttributes = &b
	}
	if q.CopyMesssageGroupID == nil {
		b := defaultCopyMesssageGroupID
		q.CopyMesssageGroupID = &b
	}
	if q.EmptyReceiveCooldown < 1 {
		q.EmptyReceiveCooldown = defaultEmptyReceiveCooldown
	}
	if q.ReceiveErrorCooldown < 1 {
		q.ReceiveErrorCooldown = defaultReceiveErrorCooldown
	}
	if q.PublishErrorCooldown < 1 {
		q.PublishErrorCooldown = defaultPublishErrorCooldown
	}
	if q.DeleteErrorCooldown < 1 {
		q.DeleteErrorCooldown = defaultDeleteErrorCooldown
	}

	return q
}
