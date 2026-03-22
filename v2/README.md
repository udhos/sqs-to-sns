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

v2 aims to reduce AWS API calls by batching both SNS Publish and SQS DeleteMessage.
So in v2 we accumulate received messages in a pool waiting for one of two events:
1 - If we accumulate 10 messages, we immediately batch-send them.
2 - If we reach a 500ms flush period without batch sends, we send what we've got
    as a partial batch.

That same accumulation logic is used twice: for the SNS Publish and for the
SQS DeleteMessage.

v2 automatically automatically sizes its goroutines.

v2 has several goroutines types:

receiver: receives SQS messages.
publisher: accumulates received messages and publishes full 10-message batches into SNS.
publisher flusher: periodically publishes messages that stalled over 500ms in accumulation.
janitor: accumulates published messages and deletes full 10-message batches from SQS.
janitor flusher: periodically deletes messages that stalled over 500ms in accumulation.

Accumulation to publish batches in SNS creates a point of attention in the total
byte size limit for the all messages in a single batch must fit into 262144 bytes.
So the pool management must take care to only pick messages that fit into that limit.

## TODO

- [X] Health check
- [X] Review flusher logic.
- [X] Delete pool (does not account payload size).
- [ ] Dogstatsd metrics
- [ ] Benchmark
- [ ] Helm chart
- [ ] Add debug logs for processed messages.
