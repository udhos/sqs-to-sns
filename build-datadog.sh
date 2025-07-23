#!/bin/bash

go install golang.org/x/vuln/cmd/govulncheck@latest
go install golang.org/x/tools/cmd/deadcode@latest
go install github.com/mgechev/revive@latest

go install github.com/DataDog/orchestrion@v1.5.0

echo "adding orchestrion pin"
orchestrion pin

gofmt -s -w .

revive ./...

go mod tidy

govulncheck ./...

deadcode ./cmd/*

go env -w CGO_ENABLED=1

go test -race ./...

go env -w CGO_ENABLED=0

orchestrion go build -o ~/go/bin/sqs-to-sns-datadog ./cmd/sqs-to-sns

go env -u CGO_ENABLED
