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
	Debug                *bool         `yaml:"debug"`
}

type config struct {
	endpointURL string

	queueListFile string
	queues        []queueConfig

	jaegerURL string

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
	debug                bool
}

func newConfig(me string) config {

	env := envconfig.NewSimple(me)

	cfg := config{

		endpointURL: env.String("ENDPOINT_URL", ""),

		// per-queue values
		queueListFile: env.String("QUEUES", "queues.yaml"),

		jaegerURL: env.String("JAEGER_URL", "http://jaeger-collector:14268/api/traces"),

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
		writers:              env.Int("WRITERS", 10), // recommended: 10*readers
		buffer:               env.Int("BUFFER", 20),  // recommended: 20*readers
		errorCooldownRead:    env.Duration("READ_ERROR_COOLDOWN", 10*time.Second),
		errorCooldownWrite:   env.Duration("WRITE_ERROR_COOLDOWN", 10*time.Second),
		errorCooldownDelete:  env.Duration("DELETE_ERROR_COOLDOWN", 10*time.Second),
		emptyReceiveCooldown: env.Duration("EMPTY_RECEIVE_COOLDOWN", 10*time.Second),
		copyAttributes:       env.Bool("COPY_ATTRIBUTES", true),
		debug:                env.Bool("DEBUG", true),
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
	if q.Debug == nil {
		b := cfg.debug
		q.Debug = &b
	}
	return q
}
