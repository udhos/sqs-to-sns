#!/bin/bash

app=sqs-to-sns

version=$(go run ./cmd/$app -version | awk '{ print $2 }' | awk -F= '{ print $2 }')

dd=-datadog

echo version=$version

latest=udhos/$app:2${dd}

docker build --no-cache \
    -t $latest \
    -t udhos/$app:$version${dd} \
    -f docker/Dockerfile.datadog .

echo push:
echo "docker push udhos/$app:$version${dd}; docker push $latest" > docker-push-datadog.sh
chmod a+rx docker-push-datadog.sh
echo docker-push-datadog.sh:
cat docker-push-datadog.sh
