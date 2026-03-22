package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type health struct {
	lastHeartbeat time.Time
	mu            sync.Mutex
	server        *http.Server
}

func (h *health) heartbeat() {
	now := time.Now()
	h.mu.Lock()
	h.lastHeartbeat = now
	h.mu.Unlock()
}

func (h *health) getLastHeartbeat() time.Time {
	h.mu.Lock()
	last := h.lastHeartbeat
	h.mu.Unlock()
	return last
}

func (h *health) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := h.server.Shutdown(ctx)
	if err != nil {
		slog.Error("health server shut down", "error", err)
		return
	}
	slog.Info("health server shut down")
}

func newHealthServer(addr, path string) *health {

	infof("health server starting: %s %s", addr, path)

	mux := http.NewServeMux()

	server := &http.Server{Addr: addr, Handler: mux}

	h := health{
		server: server,

		// In the worst case, the root long-poll SQS receiver will
		// deliver the first heartbeat after 20s.
		// We optimistically set an artificial heartbeat now to
		// prevent a 500 error during the first poll.
		// If the receiver is broken, the health check endpoint
		// will start returning 500s after the 30s limit is exceeded.
		lastHeartbeat: time.Now(),
	}

	mux.HandleFunc(path, func(w http.ResponseWriter, _ /*r*/ *http.Request) {
		lastHeartbeat := h.getLastHeartbeat()
		elapsed := time.Since(lastHeartbeat)
		const limit = 30 * time.Second
		if elapsed > limit {
			msg := fmt.Sprintf("500 health failure - last heartbeat at %v > limit=%v\n",
				elapsed, limit)
			http.Error(w, msg, 500)
			return
		}

		msg := fmt.Sprintf("200 health ok - last heartbeat at %v < limit=%v\n",
			elapsed, limit)
		io.WriteString(w, msg)
	})

	go func() {
		if err := server.ListenAndServe(); err != nil {
			errorf("health server exite with error: addr=%s %v", addr, err)
		}
	}()

	return &h
}
