
https://github.com/Admiral-Piett/goaws

```bash
git clone https://github.com/Admiral-Piett/goaws
cd goaws
go build -o ~/go/bin/goaws ./app/cmd


goaws -config app/conf/goaws.yaml


aws --endpoint-url http://localhost:4100 sqs create-queue --queue-name test1
{
    "QueueUrl": "http://us-east-1.goaws.com:4100/100010001000/test1"
}


aws --endpoint-url http://localhost:4100 sqs send-message --queue-url http://localhost:4100/test1 --message-body "this is a test of the GoAws Queue messaging"


aws --endpoint-url http://localhost:4100 sns create-topic --name test1
{
    "TopicArn": "arn:aws:sns:us-east-1:100010001000:test1"
}

aws --endpoint-url http://localhost:4100 sqs send-message --queue-url http://localhost:4100/test1 --message-body "this is a test of the GoAws Queue messaging"

more queues.yaml.goaws 
- id: q1
  queue_url: http://us-east-1.goaws.com:4100/100010001000/test1
  topic_arn: arn:aws:sns:us-east-1:100010001000:test1

export QUEUES=queues.yaml.goaws

export READERS=1
export WRITERS=1
export BUFFER=1

export ENDPOINT_URL=http://localhost:4100

sqs-to-sns
```
