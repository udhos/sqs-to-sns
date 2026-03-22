# sqs-to-sns

# How v2 differs from v1

## v1

v1 long-polls SQS for 20s using ReceiveMessage with MaxNumberOfMessages=10,
but SNS Publish sends 1 message and SQS DeleteMessage also sends 1 message.

v1 requires fine-tuning of number of reader and writer goroutines.

v1 writer goroutine performs both SNS Publish and SQS DeleteMessage.

## v2

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

v2 automatically sizes its goroutines.

v2 has several goroutines types:

receiver: receives SQS messages.
publisher: accumulates received messages and publishes full 10-message batches into SNS.
publisher flusher: periodically publishes messages that stalled over 500ms in accumulation.
                   successfully published messages are forwarded to the janitor.
janitor: accumulates published messages and deletes full 10-message batches from SQS.
janitor flusher: periodically deletes messages that stalled over 500ms in accumulation.

Accumulation to publish batches in SNS creates a point of attention in the total
byte size limit for the all messages in a single batch must fit into 262144 bytes.
So the pool management must take care to only pick messages that fit into that limit.

## TODO

- [X] Health check
- [X] Review flusher logic.
- [X] Delete pool (does not account payload size).
- [X] Log details for partial batch failed items in publish and delete.
- [ ] Add queue URL or topic ARN to logs?
- [X] Add 30s timeout with context to AWS API calls in order to prevent permanent loss of goroutine stuck on API.
- [X] Review cooldown on API errors (do not hammer API that is returning error).
- [ ] Add Dogstatsd metrics. Only the root goroutines manages metrics. It is a kind of sampling per layer: only the root layer is sampled, nonroot siblings are not. Important metrics: gauge channel size (min,avg,max), gauge amount of goroutines per type (min,avg,max), count aws api errors. How to gauge min,avg,max for fixed intervals is not yet clear.
- [ ] Run benchmark on staging environment.
- [ ] Add helm chart.
- [X] Add debug logs for processed messages.
- [ ] Document that all batching and flushing paths ultimately write to bounded/buffered channels, ensuring that backpressure is preserved and propagates through the entire pipeline.
- [ ] Document how shutdown only stops receivers and everything else is kept running in order to drain messages. No channel is closed. No other goroutine returns.
- [ ] Document the root goroutines are eternal while nonroot goroutines are created dynamically by root goroutines and eventually die (return) when detect low demand (incoming channel size below low watermark).
