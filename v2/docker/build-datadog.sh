#!/bin/bash

app=sqs-to-sns

version=$(go run ./cmd/$app -version | awk '{ print $2 }' | awk -F= '{ print $2 }')

dd=-datadog

echo version=$version

docker build --no-cache \
    -t udhos/$app:latest${dd} \
    -t udhos/$app:$version${dd} \
    -f docker/Dockerfile.datadog .

echo push:
echo "docker push udhos/$app:$version${dd}; docker push udhos/$app:latest${dd}" > docker-push-datadog.sh
chmod a+rx docker-push-datadog.sh
echo docker-push-datadog.sh:
cat docker-push-datadog.sh
