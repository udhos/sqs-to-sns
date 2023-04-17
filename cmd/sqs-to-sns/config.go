package main

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/udhos/boilerplate/envconfig"
	"github.com/udhos/boilerplate/secret"
	"gopkg.in/yaml.v2"
)

type queueConfig struct {
	ID string `yaml:"id"`

	// per-queue values
	QueueURL            string        `yaml:"queue_url"`
	QueueRoleArn        string        `yaml:"queue_role_arn"`
	TopicArn            string        `yaml:"topic_arn"`
	TopicRoleArn        string        `yaml:"topic_role_arn"`
	Readers             int           `yaml:"readers"`
	Writers             int           `yaml:"writers"`
	Buffer              int           `yaml:"buffer"`
	ErrorCooldownRead   time.Duration `yaml:"error_cooldown_read"`
	ErrorCooldownWrite  time.Duration `yaml:"error_cooldown_write"`
	ErrorCooldownDelete time.Duration `yaml:"error_cooldown_delete"`
	CopyAttributes      *bool         `yaml:"copy_attributes"`
	Debug               *bool         `yaml:"debug"`
}

type config struct {
	queueListFile string
	queues        []queueConfig

	metricsAddr      string
	metricsPath      string
	metricsNamespace string

	// default values
	queueRoleArn        string
	topicRoleArn        string
	readers             int
	writers             int
	buffer              int
	errorCooldownRead   time.Duration
	errorCooldownWrite  time.Duration
	errorCooldownDelete time.Duration
	copyAttributes      bool
	debug               bool
}

func newConfig(me string) config {

	env := getEnv(me)

	cfg := config{
		// per-queue values
		queueListFile: env.String("QUEUES", "queues.yaml"),

		metricsAddr:      env.String("METRICS_ADDR", ":3000"),
		metricsPath:      env.String("METRICS_PATH", "/metrics"),
		metricsNamespace: env.String("METRICS_NAMESPACE", "sqstosns"),

		// default values
		queueRoleArn:        env.String("QUEUE_ROLE_ARN", ""),
		topicRoleArn:        env.String("TOPIC_ROLE_ARN", ""),
		readers:             env.Int("READERS", 1),
		writers:             env.Int("WRITERS", 1),
		buffer:              env.Int("BUFFER", 10),
		errorCooldownRead:   env.Duration("READ_ERROR_COOLDOWN", 10*time.Second),
		errorCooldownWrite:  env.Duration("WRITE_ERROR_COOLDOWN", 10*time.Second),
		errorCooldownDelete: env.Duration("DELETE_ERROR_COOLDOWN", 10*time.Second),
		copyAttributes:      env.Bool("COPY_ATTRIBUTES", true),
		debug:               env.Bool("DEBUG", true),
	}

	cfg.queues = loadQueueConf(cfg)

	return cfg
}

func getEnv(me string) *envconfig.Env {
	roleArn := os.Getenv("SECRET_ROLE_ARN")

	log.Printf("SECRET_ROLE_ARN='%s'", roleArn)

	secretOptions := secret.Options{
		RoleSessionName: me,
		RoleArn:         roleArn,
	}
	secret := secret.New(secretOptions)
	envOptions := envconfig.Options{
		Secret: secret,
	}
	env := envconfig.New(envOptions)
	return env
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
