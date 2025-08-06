package main

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/udhos/boilerplate/envconfig"
	"gopkg.in/yaml.v3"
)

type queueConfig struct {
	ID string `yaml:"id"`

	// per-queue values
	QueueURL             string        `yaml:"queue_url"`
	QueueRoleArn         string        `yaml:"queue_role_arn"`
	TopicArn             string        `yaml:"topic_arn"`
	TopicRoleArn         string        `yaml:"topic_role_arn"`
	Readers              int           `yaml:"readers"`
	Writers              int           `yaml:"writers"`
	Buffer               int           `yaml:"buffer"`
	ErrorCooldownRead    time.Duration `yaml:"error_cooldown_read"`
	ErrorCooldownWrite   time.Duration `yaml:"error_cooldown_write"`
	ErrorCooldownDelete  time.Duration `yaml:"error_cooldown_delete"`
	EmptyReceiveCooldown time.Duration `yaml:"empty_receive_cooldown"`
	CopyAttributes       *bool         `yaml:"copy_attributes"`
	CopyMesssageGroupID  *bool         `yaml:"copy_message_group_id"`
	Debug                *bool         `yaml:"debug"`
	MaxNumberOfMessages  *int32        `yaml:"max_number_of_messages"` // 1..10 (default 10)
	WaitTimeSeconds      *int32        `yaml:"wait_time_seconds"`      // 0..20 (default 20)
}

type config struct {
	endpointURL string

	queueListFile string
	queues        []queueConfig

	jaegerEnable            bool
	ignoreSqsAttributeLimit bool
	prometheusEnable        bool
	dogstatsdEnable         bool
	dogstatsdDebug          bool

	healthAddr string
	healthPath string

	metricsAddr           string
	metricsPath           string
	metricsNamespace      string
	metricsBucketsLatency []float64

	// default values
	queueRoleArn         string
	topicRoleArn         string
	readers              int
	writers              int
	buffer               int
	errorCooldownRead    time.Duration
	errorCooldownWrite   time.Duration
	errorCooldownDelete  time.Duration
	emptyReceiveCooldown time.Duration
	copyAttributes       bool
	copyMesssageGroupID  bool
	debug                bool
	maxNumberOfMessages  int32 // 1..10 (default 10)
	waitTimeSeconds      int32 // 0..20 (default 20)
}

func newConfig(me string) config {

	env := envconfig.NewSimple(me)

	cfg := config{

		endpointURL: env.String("ENDPOINT_URL", ""),

		// per-queue values
		queueListFile: env.String("QUEUES", "queues.yaml"),

		jaegerEnable:            env.Bool("JAEGER_ENABLE", false),
		ignoreSqsAttributeLimit: env.Bool("IGNORE_SQS_ATTRIBUTE_LIMIT", false),
		prometheusEnable:        env.Bool("PROMETHEUS_ENABLE", true),
		dogstatsdEnable:         env.Bool("DOGSTATSD_ENABLE", true),
		dogstatsdDebug:          env.Bool("DOGSTATSD_DEBUG", false),

		healthAddr: env.String("HEALTH_ADDR", ":8888"),
		healthPath: env.String("HEALTH_PATH", "/health"),

		metricsAddr:           env.String("METRICS_ADDR", ":3000"),
		metricsPath:           env.String("METRICS_PATH", "/metrics"),
		metricsNamespace:      env.String("METRICS_NAMESPACE", "sqstosns"),
		metricsBucketsLatency: env.Float64Slice("METRICS_BUCKETS_LATENCY", []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0}),

		// default values
		queueRoleArn:         env.String("QUEUE_ROLE_ARN", ""),
		topicRoleArn:         env.String("TOPIC_ROLE_ARN", ""),
		readers:              env.Int("READERS", 1),
		writers:              env.Int("WRITERS", 15), // recommended: 15*readers
		buffer:               env.Int("BUFFER", 30),  // recommended: 30*readers
		errorCooldownRead:    env.Duration("READ_ERROR_COOLDOWN", 10*time.Second),
		errorCooldownWrite:   env.Duration("WRITE_ERROR_COOLDOWN", 10*time.Second),
		errorCooldownDelete:  env.Duration("DELETE_ERROR_COOLDOWN", 10*time.Second),
		emptyReceiveCooldown: env.Duration("EMPTY_RECEIVE_COOLDOWN", 10*time.Second),
		copyAttributes:       env.Bool("COPY_ATTRIBUTES", true),
		copyMesssageGroupID:  env.Bool("COPY_MESSAGE_GROUP_ID", true),
		debug:                env.Bool("DEBUG", true),
		maxNumberOfMessages:  int32(env.Int("MAX_NUMBER_OF_MESSAGES", 10)), // 1..10 (default 10)
		waitTimeSeconds:      int32(env.Int("WAIT_TIME_SECONDS", 20)),      // 0..20 (default 20)
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
		log.Fatalf("%s: load queues: %s: %v",
			me, queuesFile, errRead)
	}
	errYaml := yaml.Unmarshal(buf, &queues)
	if errYaml != nil {
		log.Fatalf("%s: parse yaml: %s: %v",
			me, queuesFile, errYaml)
	}
	for i, q := range queues {
		queues[i] = queueDefaults(q, cfg)
		log.Printf("queue %s: %s", q.ID, toJSON(queues[i]))
	}
	return queues
}

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func queueDefaults(q queueConfig, cfg config) queueConfig {
	if q.QueueRoleArn == "" {
		q.QueueRoleArn = cfg.queueRoleArn
	}
	if q.TopicRoleArn == "" {
		q.TopicRoleArn = cfg.topicRoleArn
	}
	if q.Readers < 1 {
		q.Readers = cfg.readers
	}
	if q.Writers < 1 {
		q.Writers = cfg.writers
	}
	if q.Buffer < 1 {
		q.Buffer = cfg.buffer
	}
	if q.ErrorCooldownRead == 0 {
		q.ErrorCooldownRead = cfg.errorCooldownRead
	}
	if q.ErrorCooldownWrite == 0 {
		q.ErrorCooldownWrite = cfg.errorCooldownWrite
	}
	if q.ErrorCooldownDelete == 0 {
		q.ErrorCooldownDelete = cfg.errorCooldownDelete
	}
	if q.EmptyReceiveCooldown == 0 {
		q.EmptyReceiveCooldown = cfg.emptyReceiveCooldown
	}
	if q.CopyAttributes == nil {
		b := cfg.copyAttributes
		q.CopyAttributes = &b
	}
	if q.CopyMesssageGroupID == nil {
		b := cfg.copyMesssageGroupID
		q.CopyMesssageGroupID = &b
	}
	if q.Debug == nil {
		b := cfg.debug
		q.Debug = &b
	}
	if q.MaxNumberOfMessages == nil {
		b := cfg.maxNumberOfMessages
		q.MaxNumberOfMessages = &b
	}
	if q.WaitTimeSeconds == nil {
		b := cfg.waitTimeSeconds
		q.WaitTimeSeconds = &b
	}
	return q
}
