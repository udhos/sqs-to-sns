[![license](http://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/udhos/sqs-to-sns/blob/main/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/udhos/sqs-to-sns)](https://goreportcard.com/report/github.com/udhos/sqs-to-sns)
[![Go Reference](https://pkg.go.dev/badge/github.com/udhos/sqs-to-sns.svg)](https://pkg.go.dev/github.com/udhos/sqs-to-sns)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/sqs-to-sns)](https://artifacthub.io/packages/search?repo=sqs-to-sns)
[![Docker Pulls](https://img.shields.io/docker/pulls/udhos/sqs-to-sns)](https://hub.docker.com/r/udhos/sqs-to-sns)

# sqs-to-sns

sqs-to-sns is an utility written in Go to forward messages from AWS SQS Queues to AWS SNS Topics. 

# TODO

- [X] Forward messages.
- [X] Docker image.
- [ ] Helm chart.
- [ ] Metrics.
- [ ] Message attributes?

# Build

```
./build.sh
```

# Env vars

```
# Mandatory
export QUEUE_URL=https://sqs.us-east-1.amazonaws.com/111111111111/queue_name
export TOPIC_ARN=arn:aws:sns:us-east-1:222222222222:topic_name

# Optional
export ROLE_ARN_SQS=arn:aws:iam::111111111111:role/sqs_consumer
export ROLE_ARN_SNS=arn:aws:iam::222222222222:role/sns_producer
export READERS=1
export WRITERS=1
export BUFFER=10
export READ_ERROR_COOLDOWN=10s
export WRITE_ERROR_COOLDOWN1=10s
```

# Roles

You can use `$ROLE_ARN_SQS` to specify a role to access the source queue, and `$ROLE_ARN_SNS` to specify a role to access the destination topic.

The role in `$ROLE_ARN_SQS` must allow actions `sqs:ReceiveMessage` and `sqs:DeleteMessage` to source queue.

The role in `$ROLE_ARN_SNS` must allow action `sns:Publish` to destination topic.

# Docker

Docker hub:

https://hub.docker.com/r/udhos/sqs-to-sns

Run from docker hub:

```
docker run -p 8080:8080 --rm udhos/sqs-to-sns:0.0.0
```

Build recipe:

```
./docker/build.sh

docker push udhos/sqs-to-sns:0.0.0
```