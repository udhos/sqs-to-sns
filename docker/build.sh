#!/bin/bash

version=$(go run ./cmd/sqs-to-sns -version | awk '{ print $2 }' | awk -F= '{ print $2 }')

echo version=$version

docker build --no-cache \
    -t udhos/sqs-to-sns:latest \
    -t udhos/sqs-to-sns:$version \
    -f docker/Dockerfile .
