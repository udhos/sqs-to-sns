#!/bin/bash

app=sqs-to-sns

version=$(go run ./cmd/$app -version | awk '{ print $2 }' | awk -F= '{ print $2 }')

echo version=$version

latest=udhos/$app:2

docker build --no-cache \
    -t $latest \
    -t udhos/$app:$version \
    -f docker/Dockerfile .

echo push:
echo "docker push udhos/$app:$version; docker push $latest" > docker-push.sh
chmod a+rx docker-push.sh
echo docker-push.sh:
cat docker-push.sh
