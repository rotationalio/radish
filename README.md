# Radish

[![GoDoc](https://godoc.org/github.com/kansaslabs/radish?status.svg)](https://godoc.org/github.com/kansaslabs/radish)
[![Go Report Card](https://goreportcard.com/badge/github.com/kansaslabs/radish)](https://goreportcard.com/report/github.com/kansaslabs/radish)
[![Build Status](https://travis-ci.com/kansaslabs/radish.svg?branch=master)](https://travis-ci.com/kansaslabs/radish)

Radish is a stateless asynchronous task queue and handler framework. Radish is designed to maximize the resources of a single node by being able to flexibly increase and decrease the number of worker go routines that handle tasks. A radish server allows users to scale the number of workers that can handle generic tasks, add tasks to the queue, and reports metrics to prometheus for easy tracking and management. Radish also provides a CLI program for interacting with servers that are running the radish service.

Radish is intended to be used as a framework to create asynchronous task handling services that do not rely on an intermediate message broker like RabbitMQ or Redis. The statelessness of Radish makes it much simpler to use, but also does not guarantee fault tolerance in task handling. It is up to the application using Radish to determine how to handle task scheduling and timeouts as well as success and failure callbacks. The way applications do this is by defining tasks handlers that implement the Task interface and registering them with the radish server. Tasks can then be queued using the Delay method or by submitting a Queue request to the API server. On success or failure, the worker will call one of the handlers callback methods then move on to the next task.

## Task Handlers

A task handler is implemented by defining a struct that implements the `Task` interface and registering it with the radish task queue. Custom tasks must specify a `Name()` method that uniquely identifies the type of task it is (which is also used when queueing tasks) as well as a `Handle()` method. The `Handle()` method must accept a uuid, which describes the future being handled (in case the application wants to implement statefulness) as well as generic parameters as a byte slice. We have chosen `[]byte` for parameters so that applications can define any serialization format they choose, e.g. json or protobuf.

```go
type SendEmail struct {}

func (t *SendEmail) Name() string {
    return "sendEmail"
}

func (t *SendEmail) Handle(id uuid.UUID, params []byte) error {}

func (t *SendEmail) Success(id uuid.UUID, params []byte) {}

func (t *SendEmail) Failure(id uuid.UUID, err error, params []byte) {}
```

Task handlers may also implement two callbacks: `Success()` and `Failure()`. Both of these callbacks take parameters that are specific to those methods and must be provided with the task being queued. The `Failure()` method will additionally be passed the error that caused the task to fail.

## Radish Quick Start

Once we have defined our custom task handlers, we can register them and begin delaying
tasks for asynchronous processing. If we have two task handlers, `SendEmail` and
`DailyReport` whose names are `"sendEmail"` and `"dailyReport"` respectively, then the
simplest way we can get started is as follows:

```go
queue, err := radish.New(nil, new(SendEmail), new(DailyReport))
id, err := queue.Delay("sendEmail", []byte("jdoe@example.com"), nil, nil)
id, err := queue.Delay("dailyReport", []byte("2020-04-07"), nil, nil)
```

When the task queue is created, it immediately launches workers (1 per CPU on the
machine) to start handling tasks. You can then delay tasks, which will return the unique
id of the future of the task (which you can use for book keeping in success or failure).
In this example, the tasks are submited with an email and an address, but no parameters
for success or failure handling.

### Configuring Radish

More detailed configuration and registration is possible with radish. In the quick start example we submitted a `nil` configuration as the first argument to `New()` - this allowed us to set reasonable defaults for the radish queue. We can configure it more specifically using the `Config` object:

```go
config := &radish.Config{Workers: 4, QueueSize: 10000}
queue, err := radish.New(config)
```

The config is validated when it is created and any invalid configurations will return an
error when the queue is created. We can also manually register tasks with the queue (and
register tasks at runtime) as follows:

```go
err := queue.Register(new(SendEmail))
```

This allows the queue to be dynamic and handle different tasks at different times. It
is also possible to scale the number of workers at runtime:

```go
queue.AddWorkers(8)
queue.RemoveWorkers(2)
queue.SetWorkers(4)
queue.NumWorkers()
```

The queue can also be scaled and tasks delayed using the Radish service.

### Radish Service

Radish implements a gRPC API so that remote clients can connect and get the queue
status, delay tasks, and scale the number of workers. The simplest way to run this
service is as follows:

```go
queue.Listen()
```

This wil serve on the address and port specified in the configuration and block until
an interrupt signal is received from the OS, which will shutdown the queue. Applications
can also manually call:

```go
queue.Shutdown()
```

To gracefully shutdown the queue, completing any tasks that are in flight and not
accepting new tasks if they run the listener in its own go routine. Applications that
need to specify their own services using gRPC or http servers can manually run the
service as follows:

```go
sock, err := net.Listen("tcp", "0.0.0.0:80")
srv := grpc.NewSever()
api.RegisterRadishServer(srv, queue)
// Register additional gRPC services here

srv.Serve(sock)
```

The radish CLI command can then be used to access the service and submit tasks.

### Metrics

Radish also serves a metrics endpoint that can be polled by Prometheus. Radish keeps track of the following metrics associated with the task queue:

- **radish.workers**: A gauge that tracks the number of workers over time as users issue scale requests.
- **radish.queue_size**: A gauge that tracks the number of the tasks in the queue currently awaiting handling.
- **radish.percent_full**: A gauge that tracks the relative fullness of the task queue based on the configured queue size.
- **radish.tasks_succeeded**: A counter that tracks the number of tasks that have been handled and succeeded, labeled by task name.
- **radish.tasks_failed**: A counter that tracks the number of tasks that have been handled and failed, labeled by task name.
- **radish.task_latency**: A histogram that tracks the amount of time it takes to handle the task and its success or failure callback in milliseconds; labeled by task name and result (success or failure).

**Coming soon:** If you have your own Prometheus endpoint, you will be able to register Radish metrics manually without serving them in Radish.

## Radish CLI

The `radish` CLI utility is found in `cmd/radish` and can be installed as follows:

```
$ go get github.com/kansaslabs/radish/cmd/radish
```

This utility allows you to interact with _any_ radish server and can be used to manage your task queue services out of the box. You can view the commands and options using `radish --help`. In order to connect to a radish server you need to specify options as follows:

```
$ radish -a localhost:5356 -U
```

This connects radish to a server on port 5356 on the local host without TLS (the `-U` stands for "unsecure"). Note that you can also use the `$RADISH_ENDPOINT` and `$RADISH_UNSECURE` environment variables.

> The misspelling of "unsecure" is a joke, radish is not insecure it's just not connecting with encryption.

After the connection options are specified you can use a command to interact with the server. For example to set the number of workers you can use the `scale` command:

```
$ radish -a localhost:5356 -U scale -w 12
```

To get the status of the server and the currently registered tasks you can use the `status` command:

```
$ radish -a localhost:5356 -U status
```

Finally, once you know the names of the tasks that the radish server is handling, you can queue tasks as follows:

```
$ radish -a localhost:5356 -U queue -t mytask -p '{"my": "data"}'
```

The CLI interface is meant to help you get quickly started with Radish task queues without having to write your own interfaces or servers.

## Turnip

An example metrics server with tasks that simply wait and have a random chance of failure is defined in `cmd/turnip`. This server is also used to benchmark Radish performance and throughput with variable length tasks. See the `examples/README.md` for more on how to get started with Turnip.

To build the Turnip image ensure you're in the root of the repository:

```
$ docker build -t kansaslabs/turnip:latest -f examples/Dockerfile .
```

KansasLabs administrators can then push this image to Dockerhub as follows:

```
$ docker push kansaslabs/turnip:latest
```
