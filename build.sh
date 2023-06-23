#!/bin/bash

go install golang.org/x/vuln/cmd/govulncheck@latest

gofmt -s -w .

revive ./...

go mod tidy

govulncheck ./...

go -w CGO_ENABLED=1

go test -race ./...

go -w CGO_ENABLED=0

go install ./...

go -u CGO_ENABLED
