/*
Package radish is a stateless ansyncrhonous task queue and handler framework that can
maximize the resources of a single node by increasing and decreasing the number of
worker go routines that can handle tasks. The radish server allows users to scale the
number of workers that can handle generic tasks, add tasks to the queue, and reports
metrics to prometheus for easy tracking and management.
*/
package radish

import (
	"sync"

	"github.com/pborman/uuid"
)

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

	return nil
}

// NumWorkers returns the number of currently running workers
func (r *Radish) NumWorkers() int {
	r.RLock()
	defer r.RUnlock()
	return len(r.workers)
}

// Handler is a thread-safe mechanism to fetch a task handler or check if it exists.
func (r *Radish) Handler(task string) (handler Task, err error) {
	r.RLock()
	defer r.Unlock()

	var ok bool
	if handler, ok = r.handlers[task]; !ok {
		return nil, Errorf(ErrTaskNotRegistered, "unknown task %q", task)
	}

	return handler, nil
}
