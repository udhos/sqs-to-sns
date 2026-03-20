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
	"syscall"
	"time"

	_ "github.com/KimMachineGun/automemlimit"
	"github.com/udhos/boilerplate/boilerplate"
	"github.com/udhos/boilerplate/envconfig"
	"github.com/udhos/sqs-to-sns/v2/internal/snsclient"
	"github.com/udhos/sqs-to-sns/v2/internal/sqsclient"
	"gopkg.in/yaml.v3"
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

	{
		fmt.Printf("configuration queues: %s\n", cfg.queueListFile)
		data, _ := yaml.Marshal(cfg.queues)
		fmt.Println(string(data))
	}

	app := newApp(cfg,

		// this client generator is called by every queue
		// to generate its clients.
		func(queueCfg queueConfig) (receiver, publisher, deleter) {

			snsClient := snsclient.NewClient(me, queueCfg.TopicArn, queueCfg.QueueRoleArn, cfg.endpointURL)
			sqsClient := sqsclient.NewClient(me, queueCfg.QueueURL, queueCfg.QueueRoleArn, cfg.endpointURL)

			return &receiverReal{sqsClient: sqsClient},
				&publisherReal{snsClient: snsClient},
				&deleterReal{sqsClient: sqsClient}
		})

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
