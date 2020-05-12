/*
The radish cli program is a utility for interacting with the radish service. For most
applications, this CLI interface allows you to delay tasks, check on the status of the
task queue, and scale the radish service.
*/
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/kansaslabs/radish"
	"github.com/kansaslabs/radish/api"
	"github.com/kansaslabs/x/noplog"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
)

func init() {
	// Stop verbose grpc logging
	grpclog.SetLogger(noplog.New())
}

var (
	conn   *grpc.ClientConn
	client api.RadishClient
)

func main() {
	// Load the .env file if exists
	godotenv.Load()

	// Instantiate the CLI application
	app := cli.NewApp()
	app.Name = "radish"
	app.Version = radish.PackageVersion
	app.Usage = "client for radish services"
	app.Before = connect
	app.After = cleanup
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "a, addr",
			Usage:  "address of the radish service to connect to",
			Value:  "localhost:5356",
			EnvVar: "RADISH_ENDPOINT",
		},
		cli.DurationFlag{
			Name:   "T, timeout",
			Usage:  "timeout before canceling request",
			Value:  30 * time.Second,
			EnvVar: "RADISH_TIMEOUT",
		},
		cli.BoolFlag{
			Name:   "U, unsecure",
			Usage:  "do not connect with TLS, connect unsecure",
			EnvVar: "RADISH_INSECURE",
		},
	}

	// Define commands available to the application
	app.Commands = []cli.Command{
		{
			Name:     "queue",
			Usage:    "enqueue a task with the specified parameters",
			Action:   queue,
			Category: "radish",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "t, task",
					Usage: "name of the task to enqueue",
				},
				cli.StringFlag{
					Name:  "p, params",
					Usage: "parameters to pass to the handler",
				},
				cli.StringFlag{
					Name:  "s, success",
					Usage: "parameters to pass to the success callback",
				},
				cli.StringFlag{
					Name:  "f, failure",
					Usage: "parameters to pass to the failure callback",
				},
			},
		},
		{
			Name:     "scale",
			Usage:    "scale the number of workers handling tasks",
			Action:   scale,
			Category: "radish",
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "w, workers",
					Usage: "set number of workers to handle tasks",
				},
			},
		},
		{
			Name:     "status",
			Usage:    "get the current status of the radish task queue",
			Action:   status,
			Category: "radish",
			Flags:    []cli.Flag{},
		},
	}

	// Run the program
	app.Run(os.Args)
}

func connect(c *cli.Context) (err error) {
	opts := make([]grpc.DialOption, 0, 2)
	opts = append(opts, grpc.WithTimeout(c.Duration("timeout")))

	if c.Bool("unsecure") {
		opts = append(opts, grpc.WithInsecure())
	} else {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	}

	if conn, err = grpc.Dial(c.String("addr"), opts...); err != nil {
		return cli.NewExitError(fmt.Errorf("could not connect to %s: %s", c.String("addr"), err), 1)
	}

	client = api.NewRadishClient(conn)
	return nil
}

func cleanup(c *cli.Context) (err error) {
	defer func() {
		conn = nil
		client = nil
	}()

	if conn != nil {
		if err = conn.Close(); err != nil {
			return cli.NewExitError(err, 1)
		}
	}
	return nil
}

func queue(c *cli.Context) (err error) {
	req := &api.QueueRequest{}

	if req.Task = c.String("task"); req.Task == "" {
		return cli.NewExitError("must specify a task name to enqueue with --task", 1)
	}

	if params := c.String("params"); params != "" {
		req.Params = []byte(params)
	}

	if success := c.String("success"); success != "" {
		req.Success = []byte(success)
	}

	if failure := c.String("failure"); failure != "" {
		req.Failure = []byte(failure)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.GlobalDuration("timeout"))
	defer cancel()

	var rep *api.QueueReply
	if rep, err = client.Queue(ctx, req); err != nil {
		return cli.NewExitError(err, 1)
	}

	return printJSONResponse(rep)
}

func scale(c *cli.Context) (err error) {
	nworkers := c.Int("workers")
	if nworkers == 0 {
		return cli.NewExitError("specify number of workers with --workers", 1)
	}

	req := &api.ScaleRequest{Workers: int32(nworkers)}
	ctx, cancel := context.WithTimeout(context.Background(), c.GlobalDuration("timeout"))
	defer cancel()

	var rep *api.ScaleReply
	if rep, err = client.Scale(ctx, req); err != nil {
		return cli.NewExitError(err, 1)
	}

	return printJSONResponse(rep)
}

func status(c *cli.Context) (err error) {
	req := &api.StatusRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), c.GlobalDuration("timeout"))
	defer cancel()

	var rep *api.StatusReply
	if rep, err = client.Status(ctx, req); err != nil {
		return cli.NewExitError(err, 1)
	}

	return printJSONResponse(rep)
}

//===========================================================================
// Helper Functions
//===========================================================================

// Prints a gRPC response as human readable json and returns cli exit error or nil.
func printJSONResponse(rep interface{}) (err error) {
	var data []byte
	if data, err = json.MarshalIndent(rep, "", " "); err != nil {
		err = fmt.Errorf("could not marshal radish response: %s", err)
		return cli.NewExitError(err, 1)
	}

	fmt.Println(string(data))
	return nil
}
