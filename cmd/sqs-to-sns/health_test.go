package main

import (
	"context"
	"net/http"
	"os"
	"sync"
	"testing"
)

// go test -race -count 1 -run TestHealth ./cmd/sqs-to-sns
func TestHealth(t *testing.T) {

	newMockSqsClient := func(_, _, _, _ string) sqsClient {
		return nil
	}

	newMockSnsClient := func(_, _, _, _ string) snsClient {
		return nil
	}

	os.Setenv("QUEUES", "/dev/null")

	app := newApp("TestHealth", newMockSqsClient, newMockSnsClient)

	server := serveHealth(app, ":8888", "/health")

	var wg sync.WaitGroup

	for range 10000 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get("http://localhost:8888/health")
			if err != nil {
				t.Errorf("error: %v", err)
				return
			}
			if resp.StatusCode != 200 {
				t.Errorf("status: %d", resp.StatusCode)
			}
		}()
	}

	wg.Wait()

	server.Shutdown(context.TODO())
}
