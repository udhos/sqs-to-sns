# sqs-to-sns v2

# Running v2

## Running from command line

```bash
# build
go install github.com/udhos/sqs-to-sns/v2/cmd/sqs-to-sns@v2.0.0

# run
sqs-to-sns
```

## Running from docker hub

```bash
docker run -p 8080:8080 --rm udhos/sqs-to-sns:2.0.0
```

## Running from helm chart

```bash
helm repo add sqs-to-sns https://udhos.github.io/sqs-to-sns

helm upgrade --install sqs-to-sns sqs-to-sns/sqs-to-sns --version 2.0.0
```

# How v2 differs from v1

## v1

v1 provides at-least-once delivery guarantee.

v1 does not use or depend on any other persistent storage.

v1 long-polls SQS for 20s using ReceiveMessage with MaxNumberOfMessages=10,
but SNS Publish sends 1 message and SQS DeleteMessage also sends 1 message.

v1 requires fine-tuning of number of reader and writer goroutines.

v1 writer goroutine performs both SNS Publish and SQS DeleteMessage.

## v2

v2 keeps at-least-once delivery guarantee.

v2 keeps independence of persistent storage.

v2 aims to be easier to configure (less knobs), more cost-effective and similarly performant.

v2 also long-polls SQS for 20s using ReceiveMessage with MaxNumberOfMessages=10.

v2 aims to reduce AWS API calls/costs by batching both SNS Publish and SQS DeleteMessage.
So in v2 we accumulate received messages in a pool waiting for one of two events:
1 - If we accumulate 10 messages, we immediately batch-send them.
2 - If we reach a 500ms flush period without full batch sends, we send what
    we've got so far as a partial batch, in order to not add excessive latency
    to messages that were delayed to build a full batch.

That same accumulation logic is used twice: for the SNS Publish and for the
SQS DeleteMessage.

v2 automatically sizes its goroutines. Goroutines scale up when channel load
exceeds 66% (high watermark) and scale down when demand drops below 33%
(low watermark). root goroutines are eternal while nonroot goroutines are
created dynamically by root goroutines and eventually die (return) when detect
low demand (incoming channel size below low watermark).

v2 has several goroutines types:

receiver: receives SQS messages.
publisher: accumulates received messages and publishes full 10-message batches into SNS.
publisher flusher: periodically publishes messages that stalled over 500ms in accumulation.
                   successfully published messages are forwarded to the janitor.
janitor: accumulates published messages and deletes full 10-message batches from SQS.
janitor flusher: periodically deletes messages that stalled over 500ms in accumulation.

Accumulation to publish batches in SNS creates a point of attention in the total
byte size limit: all messages in a single batch must fit into 262,144 bytes.
So the pool management must take care to only pick messages that fit into that limit.

SNS Batching is 'Byte-Aware'. The system tracks the cumulative size of message bodies 
and metadata, ensuring no batch exceeds the 256KB AWS limit (262,144 bytes).

# TODO

- [X] Health check
- [X] Review flusher logic.
- [X] Delete pool (does not account payload size).
- [X] Log details for partial batch failed items in publish and delete.
- [X] Add queue URL or topic ARN to logs.
- [X] Add 30s timeout with context to AWS API calls in order to prevent permanent loss of goroutine stuck on API.
- [X] Review cooldown on API errors (do not hammer API that is returning error).
- [X] Log latency.
- [X] Add Dogstatsd metrics.
- [X] Add helm chart.
- [X] Add debug logs for processed messages.
- [X] Document backpressure.
- [X] Document shutdown.
- [X] Document goroutines root/nonroot lifespan
- [X] Run benchmark on staging environment.

# Global env vars

```bash
Env Var                 Default
----------------------- ------
AUTOMEMLIMIT_DEBUG      false
LOG_LEVEL               info
LOG_JSON:               false
LOG_MESSAGE_BODY        false
QUEUES                  queues.yaml
ENDPOINT_URL            ""
EXIT_DELAY              5s
FLUSH_INTERVAL_PUBLISH  500ms
FLUSH_INTERVAL_DELETE   1s
AWS_API_TIMEOUT         30s
WATERMARK_LOW_PUBLISH   .01
WATERMARK_HIGH_PUBLISH  .01
WATERMARK_LOW_DELETE    .33
WATERMARK_HIGH_DELETE   .66
HEALTH_ADDR             :8080
HEALTH_PATH             /health
PER_MESSAGE_PADDING     500        # Orchestrion _datadog attribute adds 338-byte overhead. We add some extra room to be safe.
DOGSTATSD_ENABLE        false
DOGSTATSD_INTERVAL      20s
DOGSTATSD_NAMESPACE     sqstosns
DOGSTATSD_SAMPLE_RATE   1.0
DD_AGENT_HOST           localhost
DD_SERVICE              sqs-to-sns
```

# Per-queue configurations in queues.yaml

The env var `QUEUES` points to yaml file declaring a list of queue-to-topic mappings.

```yaml
- id: q1
  #
  # required:
  #
  queue_url: https://sqs.us-east-1.amazonaws.com/111111111111/queue_name1
  topic_arn: arn:aws:sns:us-east-1:222222222222:topic_name1
  #
  # optional:
  #
  # queue_role_arn: ""
  # topic_role_arn: ""
  # buffer_size_publish: 1000
  # buffer_size_delete: 1000
  # limit_readers: 10
  # limit_publishers: 100
  # limit_deleters: 100
  # max_number_of_messages: 10 # 1..10 (default 10)
  # wait_time_seconds: 20      # 0..20 (default 20)
  # copy_attributes: true
  # copy_message_group_id: true
  # empty_receive_cooldown: 1s
  # receive_error_cooldown: 1s
  # publish_error_cooldown: 1s
  # delete_error_cooldown: 1s
```

# Dogstatsd metrics

v2 uses a high-performance local aggregator. Every goroutine (root and sibling) records metrics into atomic buckets. A background harvester snapshots these buckets every 20s to export min, max, and avg values, ensuring even micro-bursts are captured.

Env var               | Default
--                    | --
PER_MESSAGE_PADDING   | 500        # Orchestrion _datadog attribute adds 338-byte overhead. We add some extra room to be safe.
DOGSTATSD_ENABLE      | "false"
DOGSTATSD_INTERVAL    | 20s
DOGSTATSD_NAMESPACE   | ""
DOGSTATSD_SAMPLE_RATE | "1.0"
DD_AGENT_HOST         | localhost
DD_SERVICE            | ""

Metric                 | Type                | Description
-- | -- | --
forward_latency        | Gauge (min/avg/max) | End-to-end time from SQS receive to SNS publish.
publish_channel_load   | Gauge (min/avg/max) | Buffer saturation % (Current Len / Max Cap).
delete_channel_load    | Gauge (min/avg/max) | Buffer saturation % (Current Len / Max Cap).
receiver_goroutines    | Gauge (min/avg/max) | Active receiver goroutines.
publisher_goroutines   | Gauge (min/avg/max) | Active publisher goroutines.
janitor_goroutines     | Gauge (min/avg/max) | Active janitor goroutines.
receive_errors         | Count               | Number of SQS ReceiveMessage failures.
publish_errors         | Count               | Number of SNS PublishBatch failures.
delete_errors          | Count               | Number of SQS DeleteMessage failures.
receives               | Count               | Number of SQS ReceiveMessage API calls made.
publishes              | Count               | Number of SNS PublishBatch API calls made.
deletes                | Count               | Number of SQS DeleteMessage API calls made.
partial_publishes      | Count               | Number of SNS PublishBatch calls with partial success.
partial_deletes        | Count               | Number of SQS DeleteMessage calls with partial success.
dropped_messages       | Count               | Number of messages dropped due to payload size issues.
received_messages      | Count               | Number of messages received from SQS.
published_messages     | Count               | Number of messages successfully published to SNS.
deleted_messages       | Count               | Number of messages successfully deleted from SQS.
goroutine_spawns       | Count               | Number of goroutines spawned.
goroutine_exits        | Count               | Number of goroutines exited.

# Graceful shutdown

Shutdown only stops receivers and everything else is kept running in order to drain messages. No channel is closed. No other goroutine returns.

During SIGTERM, the application stops the SQS Readers immediately. However, Publishers and Janitors continue to run until their respective channels are empty. This ensures that any message already 'in flight' within the internal buffers is successfully published to SNS and deleted from SQS before the process exits.

# Backpressure

All batching and flushing paths ultimately write to bounded/buffered channels, ensuring that backpressure is preserved and propagates through the entire pipeline.

All internal pipelines use bounded channels (buffers). If SNS slows down, the publishCh fills up, which naturally slows down the receivers, ensuring the application memory usage remains constant regardless of traffic spikes.

# Sizing

How to size the tool for Kubernetes.

As example, we ran the tool under the conditions shown below.

```bash
Test Parameters:

AWS EC2 instance:     c6a.4xlarge
Test load:            4166 messages/sec
SQS payload size:     10,000 bytes
K8s POD CPU request:  500m
K8s POD CPU limit:    1
K8s HPA target:       80%

Test Results:

Tool e2e Latency:     min=18ms avg=36ms max=383ms
POD CPU Average:      374m
POD Mem Average:      58Mi
HPA number of PODs:   5
```

HPA would further scale up the number of replicas at 400m (80% x 500m).
Thus we know 374m is close to the scaling threshold.
From the good latency values, we can tell that each POD is not saturated to the point of degradation.
We calculate the POD capacity: 4166 messages/sec / 5 = 833 messages/sec.

Then each POD can deliver at least 833 messages/sec without degradation.

In production we want to support peaks as high as 30,000 messages/sec:

30,000 / 833 = 36 PODs

36 is the value to be used in HPA maxReplicas in order to support up to 30,000 messages/sec.
