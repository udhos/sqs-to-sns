#!/bin/bash

app=sqs-to-sns

version=$(go run ./cmd/$app -version | awk '{ print $2 }' | awk -F= '{ print $2 }')

echo version=$version

docker build --no-cache \
    -t udhos/$app:latest \
    -t udhos/$app:$version \
    -f docker/Dockerfile .

echo "push: docker push udhos/$app:$version; docker push udhos/$app:latest"
