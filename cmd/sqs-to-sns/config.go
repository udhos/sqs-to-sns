package main

import (
	"log"
	"os"
	"time"

	"github.com/udhos/boilerplate/envconfig"
	"github.com/udhos/boilerplate/secret"
)

type config struct {
	roleArnSqs         string
	queueURL           string
	roleArnSns         string
	topicArn           string
	readers            int
	writers            int
	buffer             int
	errorCooldownRead  time.Duration
	errorCooldownWrite time.Duration
}

func newConfig(me string) config {

	env := getEnv(me)

	return config{
		roleArnSqs:         env.String("ROLE_ARN_SQS", ""),
		queueURL:           env.String("QUEUE_URL", ""),
		roleArnSns:         env.String("ROLE_ARN_SNS", ""),
		topicArn:           env.String("TOPIC_ARN", ""),
		readers:            env.Int("READERS", 1),
		writers:            env.Int("WRITERS", 1),
		buffer:             env.Int("BUFFER", 10),
		errorCooldownRead:  env.Duration("READ_ERROR_COOLDOWN", 10*time.Second),
		errorCooldownWrite: env.Duration("WRITE_ERROR_COOLDOWN", 10*time.Second),
	}
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
