# STEP 1 build executable binary

FROM golang:1.20.3-alpine3.17 as builder

RUN apk update
RUN apk add git

# Create appuser on builder image
RUN adduser -D -g '' appuser

COPY ./cmd/ /tmp/app/cmd
COPY go.* /tmp/app/
WORKDIR /tmp/app
RUN go mod tidy
RUN go env -w CGO_ENABLED=0
RUN go build -o /tmp/sqs-to-sns github.com/udhos/sqs-to-sns/cmd/sqs-to-sns

#
# STEP 2 build a small image from alpine
#
# curl: https://security.alpinelinux.org/srcpkg/curl
#
FROM alpine:3.17.3
COPY --from=builder /tmp/sqs-to-sns /bin/sqs-to-sns
RUN apk add curl=8.0.1-r0
RUN adduser -D -g '' user
USER user
ENTRYPOINT ["/bin/sqs-to-sns"]
