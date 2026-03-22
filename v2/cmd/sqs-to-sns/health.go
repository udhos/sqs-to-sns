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

	h := health{server: server}

	mux.HandleFunc(path, func(w http.ResponseWriter, _ /*r*/ *http.Request) {
		elapsed := time.Since(h.lastHeartbeat)
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
