[![license](http://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/udhos/sqs-to-sns/blob/main/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/udhos/sqs-to-sns)](https://goreportcard.com/report/github.com/udhos/sqs-to-sns)
[![Go Reference](https://pkg.go.dev/badge/github.com/udhos/sqs-to-sns.svg)](https://pkg.go.dev/github.com/udhos/sqs-to-sns)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/sqs-to-sns)](https://artifacthub.io/packages/search?repo=sqs-to-sns)
[![Docker Pulls](https://img.shields.io/docker/pulls/udhos/sqs-to-sns)](https://hub.docker.com/r/udhos/sqs-to-sns)

# sqs-to-sns

sqs-to-sns is an utility written in Go to forward messages from AWS SQS Queues to AWS SNS Topics.

* [TODO](#todo)
* [Build and run](#build-and-run)
* [Env vars](#env-vars)
* [Roles](#roles)
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
- [ ] Metrics.
- [ ] Health check.

# Build and run

```
git clone https://github.com/udhos/sqs-to-sns

cd sqs-to-sns

./build.sh ;# compile

sts-to-sns ;# run the executable
```

# Env vars

```
#
# Mandatory
#

export QUEUE_URL=https://sqs.us-east-1.amazonaws.com/111111111111/queue_name
export TOPIC_ARN=arn:aws:sns:us-east-1:222222222222:topic_name

#
# Optional
#

export ROLE_ARN_SQS=arn:aws:iam::111111111111:role/sqs_consumer
export ROLE_ARN_SNS=arn:aws:iam::222222222222:role/sns_producer

export READERS=1                 ;# number of goroutines reading from SQS queue
export WRITERS=1                 ;# number of goroutines writing to SNS topic
export BUFFER=10                 ;# buffer size between readers and writers
export READ_ERROR_COOLDOWN=10s   ;# cooldown holdtime between read errors
export WRITE_ERROR_COOLDOWN=10s  ;# cooldown holdtime between write errors
export COPY_ATTRIBUTES=true      ;# enable copying of message attributes from SQS message to SNS message
export DEBUG=true                ;# enable debug logs
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
