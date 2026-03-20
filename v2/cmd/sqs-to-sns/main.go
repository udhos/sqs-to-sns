// Package main implements the tool.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/KimMachineGun/automemlimit"
	"github.com/udhos/boilerplate/boilerplate"
	"github.com/udhos/boilerplate/envconfig"
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
		slog.Info(v)
	}

	env := envconfig.NewSimple(me)

	//
	// log level and format
	//
	levelStr := strings.ToLower(env.String("LOG_LEVEL", "info"))
	isJSON := env.Bool("LOG_JSON", false)
	setupLogging(levelStr, isJSON)

	//
	// run application
	//

	cfg := newConfig(env)

	app := newApp(cfg, &receiverReal{}, &publisherReal{}, &deleterReal{})

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

type deleterReal struct{}

func (d *deleterReal) delete(q *queue, msg []message) error {
	return fmt.Errorf("deleterReal.delete: WRITEME: %v: %d", q, len(msg))
}

type publisherReal struct{}

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
