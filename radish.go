/*
Package radish is a stateless asynchronous task queue and handler framework. Radish is
designed to maximize the resources of a single node by being able to flexibly increase
and decrease the number of worker go routines that handle tasks. A radish server allows
users to scale the number of workers that can handle generic tasks, add tasks to the
queue, and reports metrics to prometheus for easy tracking and management. Radish also
provides a CLI program for interacting with servers that are running the radish service.

Radish is intended to be used as a framework to create asynchronous task handling
services that do not rely on an intermediate message broker like RabbitMQ or Redis. The
statelessness of Radish makes it much simpler to use, but also does not guarantee fault
tolerance in task handling. It is up to the application using Radish to determine how to
handle task scheduling and timeouts as well as success and failure callbacks. The way
applications do this is by defining tasks handlers that implement the Task interface and
registering them with the radish server. Tasks can then be queued using the Delay method
or by submitting a Queue request to the API server. On success or failure, the worker
will call one of the handlers callback methods then move on to the next task.

Task Handlers

A task handler is implemented by defining a struct that implements the Task interface
and registering it with the Radish task queue. Custom tasks must specify a Name method
that uniquely identifies the type of task it is (which is also used when queueing tasks)
as well as a Handle method. The Handle method must accept a uuid, which describes the
future being handled (in case the application wants to implement statefulness) as well
as generic parameters as a byte slice. We have chosen []byte for parameters so that
applications can define any serialization format they choose, e.g. json or protobuf.

	type SendEmail struct {}

	func (t *SendEmail) Name() string {
		return "sendEmail"
	}

	func (t *SendEmail) Handle(id uuid.UUID, params []byte) error {}

	func (t *SendEmail) Success(id uuid.UUID, params []byte) {}

	func (t *SendEmail) Failure(id uuid.UUID, err error, params []byte) {}

Task handlers may also implement two callbacks: Success and Failure. Both of these
callbacks take parameters that are specific to those methods and must be provided with
the task being queued. The Failure method will additionally be passed the error that
caused the task to fail.

Radish Quick Start

Once we have defined our custom task handlers, we can register them and begin delaying
tasks for asynchronous processing. If we have two task handlers, SendEmail and
DailyReport whose names are "sendEmail" and "dailyReport" respectively, then the
simplest way we can get started is as follows:

	queue, err := radish.New(nil, new(SendEmail), new(DailyReport))
	id, err := queue.Delay("sendEmail", []byte("jdoe@example.com"), nil, nil)
	id, err := queue.Delay("dailyReport", []byte("2020-04-07"), nil, nil)

When the task queue is created, it immediately launches workers (1 per CPU on the
machine) to start handling tasks. You can then delay tasks, which will return the unique
id of the future of the task (which you can use for book keeping in success or failure).
In this example, the tasks are submited with an email and an address, but no parameters
for success or failure handling.

Configuring Radish

More detailed configuration and registration is possible with radish. In the quick start
example we submitted a nil configuration as the first argument to New - this allowed us
to set reasonable defaults for the radish queue. We can configure it more specifically
using the Config object:

	config := &radish.Config{Workers: 4, QueueSize: 10000}
	queue, err := radish.New(config)

The config is validated when it is created and any invalid configurations will return an
error when the queue is created. We can also manually register tasks with the queue (and
register tasks at runtime) as follows:

	err := queue.Register(new(SendEmail))

This allows the queue to be dynamic and handle different tasks at different times. It
is also possible to scale the number of workers at runtime:

	queue.AddWorkers(8)
	queue.RemoveWorkers(2)
	queue.SetWorkers(4)
	queue.NumWorkers()

The queue can also be scaled and tasks delayed using the Radish service.

Radish Service

Radish implements a gRPC API so that remote clients can connect and get the queue
status, delay tasks, and scale the number of workers. The simplest way to run this
service is as follows:

	queue.Listen()

This wil serve on the address and port specified in the configuration and block until
an interrupt signal is received from the OS, which will shutdown the queue. Applications
can also manually call:

	queue.Shutdown()

To gracefully shutdown the queue, completing any tasks that are in flight and not
accepting new tasks if they run the listener in its own go routine. Applications that
need to specify their own services using gRPC or http servers can manually run the
service as follows:

	sock, err := net.Listen("tcp", "0.0.0.0:80")
	srv := grpc.NewSever()
	api.RegisterRadishServer(srv, queue)
	// Register additional gRPC services here

	srv.Serve(sock)

The radish CLI command can then be used to access the service and submit tasks.

Metrics

Radish also serves a metrics endpoint that can be polled by Prometheus.

*/
package radish

import (
	"sync"

	"github.com/kansaslabs/x/out"
	"github.com/pborman/uuid"
)

// PackageVersion of the current Raft implementation
const PackageVersion = "1.0"

// New creates a Radish object with the specified config and registers the specified
// task handlers. If the handler cannot be registered or the config is invalid an error
// is returned.
func New(config *Config, tasks ...Task) (r *Radish, err error) {
	if config == nil {
		config = new(Config)
	}

	// Validate the configuration
	if err = config.Validate(); err != nil {
		return nil, err
	}

	// Create the radish instance
	r = &Radish{
		config:   config,
		tasks:    make(chan *Future, config.QueueSize),
		workers:  make([]*worker, 0, config.Workers),
		handlers: make(map[string]Task),
	}

	// Register the tasks on the radish server
	for _, task := range tasks {
		if err = r.Register(task); err != nil {
			return nil, err
		}
	}

	// Create the workers and start them
	if err = r.AddWorkers(config.Workers); err != nil {
		return nil, err
	}

	return r, nil
}

// Radish is a stateless task queue. It listens to requests via the gRPC api to enqueue
// tasks (or they can be enqueued directly in code) and manages workers to handle each
// task in the order they are received. Before running the server, tasks must be
// registered so that the Radish queue knows how to handle them.
type Radish struct {
	sync.RWMutex                 // server concurrency control for both workers and registration
	config       *Config         // the radish configuration
	tasks        chan *Future    // the task queue that workers are operating on
	workers      []*worker       // the workers that are currently operating on the queue
	handlers     map[string]Task // all currently registered tasks the server can handle
}

// Register a task handler with the Radish task queue.
func (r *Radish) Register(task Task) (err error) {
	r.Lock()
	defer r.Unlock()

	// Check to see if a task with this name has already been registered
	if _, ok := r.handlers[task.Name()]; ok {
		return Errorf(ErrTaskAlreadyRegistered, "task named %q has already been registered", task.Name())
	}

	r.handlers[task.Name()] = task
	out.Info("registered task %s", task.Name())
	return nil
}

// Delay creates a new future and adds it to the task queue if the handler has been registered.
func (r *Radish) Delay(task string, params, success, failure []byte) (id uuid.UUID, err error) {
	if _, err = r.Handler(task); err != nil {
		return nil, Errorf(ErrTaskNotRegistered, "could not delay %s", err)
	}

	// TODO: replace uuid.NewRandom with  uuid.NewUUID?
	future := &Future{
		ID:      uuid.NewRandom(),
		Task:    task,
		Params:  params,
		Success: success,
		Failure: failure,
	}

	r.tasks <- future

	// Update the queue size and percent full
	pmQueueSize.Set(float64(len(r.tasks)))
	pmPercentFull.Set(float64(len(r.tasks)) / float64(r.config.QueueSize) * 100)
	return future.ID, nil
}

// SetWorkers to the specified number of workers. Does nothing if n == number of workers
// that are running. Adds workers if n > number of workers and removes workers if
// n > number of workers.
func (r *Radish) SetWorkers(n int) (err error) {
	if n < 0 {
		return Errorf(ErrInvalidWorkers, "cannot set number of workers <0")
	}

	r.Lock()
	defer r.Unlock()

	nworkers := len(r.workers)
	if n > nworkers {
		return r.addWorkers(n - nworkers)
	}

	if n < nworkers {
		return r.removeWorkers(nworkers - n)
	}

	return nil
}

// AddWorkers to process tasks. Note that this is thread-safe but does start go routines.
func (r *Radish) AddWorkers(n int) (err error) {
	r.Lock()
	defer r.Unlock()
	return r.addWorkers(n)
}

// add workers, not thread-safe
func (r *Radish) addWorkers(n int) (err error) {
	if n == 0 {
		return nil
	} else if n < 0 {
		return Errorf(ErrInvalidWorkers, "cannot add negative workers, use RemoveWorkers")
	}

	for i := 0; i < n; i++ {
		w := &worker{parent: r, stop: make(chan bool)}
		r.workers = append(r.workers, w)
		go w.run()
	}

	// Update the workers gauge
	pmWorkers.Set(float64(len(r.workers)))

	out.Status("added %d workers -- %d workers running", n, len(r.workers))
	return nil
}

// RemoveWorkers by stopping them gracefully after they've completed the given task.
func (r *Radish) RemoveWorkers(n int) (err error) {
	r.Lock()
	defer r.Unlock()
	return r.removeWorkers(n)
}

// remove workers, not thread-safe
func (r *Radish) removeWorkers(n int) (err error) {
	if n > len(r.workers) {
		return Errorf(ErrInvalidWorkers, "cannot remove %d workers, only %d currently running", n, len(r.workers))
	} else if n == 0 {
		return nil
	} else if n < 0 {
		return Errorf(ErrInvalidWorkers, "cannot remove negative workers, use AddWorkers")
	}

	for i := 0; i < n; i++ {
		w := len(r.workers) - 1
		r.workers[w].stop <- true // wait for worker to stop, this should block
		r.workers[w] = nil        // delete the worker
		r.workers = r.workers[:w] // truncate the workers list
	}

	// Update the workers gauge
	pmWorkers.Set(float64(len(r.workers)))

	out.Status("removed %d workers -- %d workers running", n, len(r.workers))
	return nil
}

// NumWorkers returns the number of currently running workers
func (r *Radish) NumWorkers() int {
	r.RLock()
	defer r.RUnlock()

	// Refresh the workers gauge
	pmWorkers.Set(float64(len(r.workers)))

	return len(r.workers)
}

// Handler is a thread-safe mechanism to fetch a task handler or check if it exists.
func (r *Radish) Handler(task string) (handler Task, err error) {
	r.RLock()
	defer r.RUnlock()

	var ok bool
	if handler, ok = r.handlers[task]; !ok {
		return nil, Errorf(ErrTaskNotRegistered, "unknown task %q", task)
	}

	return handler, nil
}
