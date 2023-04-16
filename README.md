[![license](http://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/udhos/sqs-to-sns/blob/main/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/udhos/sqs-to-sns)](https://goreportcard.com/report/github.com/udhos/sqs-to-sns)
[![Go Reference](https://pkg.go.dev/badge/github.com/udhos/sqs-to-sns.svg)](https://pkg.go.dev/github.com/udhos/sqs-to-sns)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/sqs-to-sns)](https://artifacthub.io/packages/search?repo=sqs-to-sns)
[![Docker Pulls](https://img.shields.io/docker/pulls/udhos/sqs-to-sns)](https://hub.docker.com/r/udhos/sqs-to-sns)

# sqs-to-sns

sqs-to-sns is an utility written in Go to forward messages from AWS SQS Queues to AWS SNS Topics.

* [TODO](#todo)
* [Build and run](#build-and-run)
* [Configuration](#configuration)
  * [Env vars](#env-vars)
  * [Queue list configuration file](#queue-list-configuration-file)
  * [Roles](#roles)
* [Prometheus Metrics](#prometheus-metrics)
* [Utility to populate SQS queue](#utility-to-populate-sqs-queue)
* [Docker](#docker)
* [Helm chart](#helm-chart)
  * [Using the repository](#using-the-repository)
  * [Create](#create)
  * [Lint](#lint)
  * [Test rendering chart templates locally](#test-rendering-chart-templates-locally)
  * [Render templates at server](#render-templates-at-server)
  * [Generate files for a chart repository](#generate-files-for-a-chart-repository)
  * [Install](#install)
  * [Upgrade](#upgrade)
  * [Uninstall](#uninstall)

Created by [gh-md-toc](https://github.com/ekalinin/github-markdown-toc.go)

# TODO

- [X] Forward messages.
- [X] Docker image.
- [X] Message attributes.
- [X] Helm chart.
- [X] Multiple queues.
- [X] Metrics.
- [ ] Health check.

# Build and run

```
git clone https://github.com/udhos/sqs-to-sns

cd sqs-to-sns

./build.sh ;# compile

sts-to-sns ;# run the executable
```

# Configuration

## Env vars

```
export QUEUES=queues.yaml ;# queue list configuration file

#
# Prometheus metrics
#

export METRICS_ADDR=:3000
export METRICS_PATH=/metrics
export METRICS_NAMESPACE=sqstosns

#
# These env vars define global defaults for per-queue config
#

export QUEUE_ROLE_ARN=arn:aws:iam::111111111111:role/sqs_consumer
export TOPIC_ROLE_ARN=arn:aws:iam::222222222222:role/sns_producer

export READERS=1                 ;# number of goroutines reading from SQS queue
export WRITERS=1                 ;# number of goroutines writing to SNS topic
export BUFFER=10                 ;# buffer size between readers and writers
export READ_ERROR_COOLDOWN=10s   ;# cooldown holdtime between read errors
export WRITE_ERROR_COOLDOWN=10s  ;# cooldown holdtime between write errors
export COPY_ATTRIBUTES=true      ;# enable copying of message attributes from SQS message to SNS message
export DEBUG=true                ;# enable debug logs
```

## Queue list configuration file

```
$ cat queues.yaml
- id: q1
  #
  # required
  #
  queue_url: https://sqs.us-east-1.amazonaws.com/111111111111/queue_name1
  topic_arn: arn:aws:sns:us-east-1:222222222222:topic_name1
  #
  # optional
  #
  #queue_role_arn: arn:aws:iam::111111111111:role/sqs_consumer1
  #topic_role_arn: arn:aws:iam::222222222222:role/sns_producer1
  #readers: 1
  #writers: 1
  #buffer: 10
  #error_cooldown_read: 10s
  #error_cooldown_write: 10s
  #copy_attributes: true
  #debug: true
- id: q2
  #
  # required
  #
  queue_url: https://sqs.us-east-1.amazonaws.com/111111111111/queue_name2
  topic_arn: arn:aws:sns:us-east-1:222222222222:topic_name2
  #
  # optional
  #
  #queue_role_arn: arn:aws:iam::111111111111:role/sqs_consumer2
  #topic_role_arn: arn:aws:iam::222222222222:role/sns_producer2
  #readers: 1
  #writers: 1
  #buffer: 10
  #error_cooldown_read: 10s
  #error_cooldown_write: 10s
  #copy_attributes: true
  #debug: true
```

## Roles

You can use the "queue role ARN" to specify a role to access the source queue, and "topic role ARN" to specify a role to access the destination topic.

The role in "queue role ARN" must allow actions `sqs:ReceiveMessage` and `sqs:DeleteMessage` to source queue.

The role in "topic role ARN" must allow action `sns:Publish` to destination topic.

# Prometheus Metrics

```
# HELP sqstosns_receive_count How many SQS receives called, partitioned by queue.
# TYPE sqstosns_receive_count counter

# HELP sqstosns_receive_error_count How many SQS receives errored, partitioned by queue.
# TYPE sqstosns_receive_error_count counter

# HELP sqstosns_receive_empty_count How many SQS empty receives, partitioned by queue.
# TYPE sqstosns_receive_empty_count counter

# HELP sqstosns_receive_messages_count How many SQS messages received, partitioned by queue.
# TYPE sqstosns_receive_messages_count counter

# HELP sqstosns_publish_error_count How many SNS publishes errored, partitioned by queue.
# TYPE sqstosns_publish_error_count counter

# HELP sqstosns_delete_error_count How many SQS deletes errored, partitioned by queue.
# TYPE sqstosns_delete_error_count counter

# HELP sqstosns_delivery_count How many SQS deliveries fully processed, partitioned by queue.
# TYPE sqstosns_delivery_count counter

# HELP sqstosns_delivery_duration_seconds How long it took to fully process the delivery, partitioned by queue.
# TYPE sqstosns_delivery_duration_seconds histogram
```

# Utility to populate SQS queue

Use `batch-sqs` to send messages to an SQS queue.

```
batch-sqs -queueURL https://sqs.us-east-1.amazonaws.com/111111111111/queue_name -count 50
```


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

# Helm chart

## Using the repository

See https://udhos.github.io/sqs-to-sns/.

## Create

```
mkdir charts
cd charts
helm create sqs-to-sns
```

Then edit files.

## Lint

```
helm lint ./charts/sqs-to-sns --values charts/sqs-to-sns/values.yaml
```

## Test rendering chart templates locally

```
helm template sqs-to-sns ./charts/sqs-to-sns --values charts/sqs-to-sns/values.yaml
```

## Render templates at server

```
helm install sqs-to-sns ./charts/sqs-to-sns --values charts/sqs-to-sns/values.yaml --dry-run
```

## Generate files for a chart repository

A chart repository is an HTTP server that houses one or more packaged charts.
A chart repository is an HTTP server that houses an index.yaml file and optionally (*) some packaged charts.

(*) Optionally since the package charts could be hosted elsewhere and referenced by the index.yaml file.

    docs
    ├── index.yaml
    └── sqs-to-sns-0.1.0.tgz

See script [update-charts.sh](update-charts.sh):

    # generate chart package from source
    helm package ./charts/sqs-to-sns -d ./docs

    # regenerate the index from existing chart packages
    helm repo index ./docs --url https://udhos.github.io/sqs-to-sns/

## Install

```
helm install sqs-to-sns ./charts/sqs-to-sns --values charts/sqs-to-sns/values.yaml
```

## Upgrade

```
helm upgrade sqs-to-sns ./charts/sqs-to-sns --values charts/sqs-to-sns/values.yaml --install
```

## Uninstall

```
helm uninstall sqs-to-sns
```
