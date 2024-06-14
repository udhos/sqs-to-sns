package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

type health struct {
	status error
	when   time.Time
}

func serveHealth(app *application, addr, path string) *http.Server {

	const me = "serveHealth"

	var lastQueue string
	var lastStatus health
	var lastFind time.Time
	var lastLock sync.Mutex

	const cacheTTL = 10 * time.Second

	mux := http.NewServeMux()

	mux.HandleFunc(path, func(w http.ResponseWriter, _ /*r*/ *http.Request) {
		var h health
		var queue string

		lastLock.Lock()

		elap := time.Since(lastFind)
		cache := elap < cacheTTL

		if cache {
			// use cached
			h = lastStatus
			queue = lastQueue
		} else {
			// scan health
			h, queue = findError(app)

			// save in cache
			lastQueue = queue
			lastStatus = h
			lastFind = time.Now()
		}

		lastLock.Unlock()

		if h.status == nil {
			//
			// healthy
			//
			io.WriteString(w, fmt.Sprintf("200 server ok (cached:%t)\n", cache))
			return
		}

		//
		// unhealthy
		//

		msg := fmt.Sprintf("500 server failing: queue:%s error:%v when:%v (cached:%t)\n",
			queue, h.status, h.when, cache)

		http.Error(w, msg, 500)
	})

	log.Printf("%s: starting health server at: %s %s", me, addr, path)

	server := &http.Server{Addr: addr, Handler: mux}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Printf("%s: addr=%s exited: %v", me, addr, err)
		}
	}()

	return server
}

func findError(app *application) (health, string) {
	for _, q := range app.queues {
		status := q.getStatus()
		//log.Printf("findError: %s: %v", q.conf.ID, status)
		if status.status != nil {
			return status, q.conf.ID // found error
		}
	}
	return health{}, "" // no error
}
