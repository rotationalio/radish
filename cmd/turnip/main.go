/*
The turnip command demonstrates how to use the Radish framework. It also serves as a
benchmarking and testing server for variable length tasks.
*/
package main

import (
	"math/rand"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/kansaslabs/radish"
	"github.com/kansaslabs/x/noplog"
	"github.com/urfave/cli"
	"google.golang.org/grpc/grpclog"
)

// Initialize the package and random numbers, etc.
func init() {
	// Set the random seed to something different each time.
	rand.Seed(time.Now().UnixNano())

	// Stop the grpc verbose logging
	grpclog.SetLogger(noplog.New())
}

func main() {
	// Load the .env file if it exists
	godotenv.Load()

	// Instantiate the CLI application
	app := cli.NewApp()
	app.Name = "turnip"
	app.Version = radish.PackageVersion
	app.Usage = "benchmarking and testing Radish server"

	// Define commands available to the application
	app.Commands = []cli.Command{
		{
			Name:     "serve",
			Usage:    "run the turnip server",
			Action:   serve,
			Category: "server",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   "a, addr",
					Usage:  "the address to bind the server on",
					Value:  ":5356",
					EnvVar: "TURNIP_ADDR",
				},
				cli.StringFlag{
					Name:   "m, metrics-addr",
					Usage:  "the address to serve prometheus metrics on",
					Value:  ":9090",
					EnvVar: "TURNIP_METRICS_ADDR",
				},
				cli.IntFlag{
					Name:   "w, workers",
					Usage:  "number of workers to start with (default is num cpus)",
					EnvVar: "TURNIP_WORKERS",
				},
				cli.IntFlag{
					Name:   "q, queue-size",
					Usage:  "size of the tasks channel",
					Value:  5000,
					EnvVar: "TURNIP_QUEUE_SIZE",
				},
				cli.StringFlag{
					Name:   "l, log-level",
					Usage:  "specify verbosity of logging (trace, debug, info, caution, status, warn, silent)",
					Value:  "info",
					EnvVar: "TURNIP_LOG_LEVEL",
				},
				cli.UintFlag{
					Name:   "c, caution-threshold",
					Usage:  "threshold before reissuing a caution message",
					Value:  50,
					EnvVar: "TURNIP_CAUTION_THRESHOLD",
				},
				cli.BoolFlag{
					Name:   "S, no-metrics",
					Usage:  "do not run the prometheus metrics server",
					EnvVar: "TURNIP_SUPPRESS_METRICS",
				},
			},
		},
	}

	// Run the program
	app.Run(os.Args)
}

func serve(c *cli.Context) (err error) {

	conf := &radish.Config{
		QueueSize:        c.Int("queue-size"),
		Workers:          c.Int("workers"),
		Addr:             c.String("addr"),
		MetricsAddr:      c.String("metrics-addr"),
		SuppressMetrics:  c.Bool("no-metrics"),
		LogLevel:         c.String("log-level"),
		CautionThreshold: c.Uint("caution-threshold"),
	}

	// Create variable length turnip tasks
	short := &Turnip{name: "short", minDelay: 50 * time.Millisecond, maxDelay: 1500 * time.Millisecond, errProb: 0.125}
	medium := &Turnip{name: "medium", minDelay: 750 * time.Millisecond, maxDelay: 5 * time.Second, errProb: 0.183}
	long := &Turnip{name: "long", minDelay: 10 * time.Second, maxDelay: 2 * time.Minute, errProb: 0.213}
	chance := &Turnip{name: "chance", minDelay: 750 * time.Millisecond, maxDelay: 2 * time.Second, errProb: 0.523}

	var srv *radish.Radish
	if srv, err = radish.New(conf, short, medium, long, chance); err != nil {
		return cli.NewExitError(err, 1)
	}

	if err = srv.Listen(); err != nil {
		return cli.NewExitError(err, 1)
	}

	return nil
}
