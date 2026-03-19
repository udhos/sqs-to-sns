// Package main implements the tool.
package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	_ "github.com/KimMachineGun/automemlimit"
	"github.com/udhos/boilerplate/boilerplate"
)

func main() {

	//
	// parse cmd line
	//

	var showVersion bool
	flag.BoolVar(&showVersion, "version", showVersion, "show version")
	flag.Parse()

	//
	// show version
	//

	me := filepath.Base(os.Args[0])

	{
		v := boilerplate.LongVersion(me + " version=" + version)
		if showVersion {
			fmt.Println(v)
			return
		}
		log.Print(v)
	}

	//
	// run application
	//

	cfg := newConfig(me)

	app := newApp(cfg, &receiverReal{}, &publisherReal{})

	app.run()

	gracefulShutdown()

	app.stopReaders()

	infof("main: sleeping %v before exiting", cfg.exitDelay)
	time.Sleep(cfg.exitDelay)

	slog.Info("main: exiting")
}

func gracefulShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	infof("received signal '%v', initiating shutdown", sig)
}

type publisherReal struct {
}

func (p *publisherReal) publish(q *queue, msg []message) ([]message, error) {
	return nil, fmt.Errorf("publisherReal.publish: WRITEME: %v: %d", q, len(msg))
}

type receiverReal struct {
	stopped bool
	mu      sync.Mutex
}

func (r *receiverReal) receive(q *queue) ([]message, bool, error) {
	r.mu.Lock()
	stopped := r.stopped
	r.mu.Unlock()

	time.Sleep(500 * time.Millisecond)
	return nil, stopped, fmt.Errorf("receiverReal.receive: WRITEME: %v", q)
}

func (r *receiverReal) stop(q *queue) error {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()

	return fmt.Errorf("receiverReal.stop: WRITEME: %v", q)
}
