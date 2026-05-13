package radish

import (
	"database/sql"
	"errors"
	"sync"
	"time"
)

var (
	ErrNoDatabase = errors.New("no database connection or configuration provided")
	ErrRunning    = errors.New("radish cannot be modified while running")
)

type Radish struct {
	mu        sync.RWMutex
	wg        *sync.WaitGroup
	conf      Config
	workers   *Workers
	conn      *sql.DB
	executors []*executor
}

type executor struct {
	conf    *Config
	workers *Workers
	conn    *sql.DB
	stop    chan<- struct{}
}

func New(conf *Config) (_ *Radish, err error) {
	// If no config is provided, load the config from the environment.
	// Otherwise, validate the config in case it wasn't loaded from the environment.
	if conf == nil {
		var cfg Config
		if cfg, err = LoadConfig(); err != nil {
			return nil, err
		}
		conf = &cfg
	} else {
		if err = conf.Validate(); err != nil {
			return nil, err
		}
	}

	// If using managed database, validate that a connection is provided.
	if conf.ManagedDB && conf.Conn == nil {
		return nil, ErrNoDatabase
	}

	return &Radish{
		conf:      *conf,
		executors: nil,
		workers: &Workers{
			workers: make(map[string]untypedWorker),
		},
	}, nil
}

// Starts the radish executors each in their own goroutine with a copy of the config
// and workers to execute tasks in parallel. Returns an error if radish is already running.
func (r *Radish) Run() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.isRunning() {
		return ErrRunning
	}

	// Connect to the database or use the provided database connection.
	if r.conf.ManagedDB {
		if r.conf.Conn == nil {
			return ErrNoDatabase
		}
		r.conn = r.conf.Conn
	} else {
		// TODO: Connect to the database using the provided database URL.
	}

	// Create a wait group to wait for all executors to finish.
	r.wg = &sync.WaitGroup{}
	r.wg.Add(r.conf.NumWorkers)

	// Create the executors with a copy of the config and workers to execute tasks in parallel.
	for i := 0; i < r.conf.NumWorkers; i++ {
		executor := &executor{conf: &r.conf, workers: r.workers, conn: r.conn}
		r.executors = append(r.executors, executor)
		executor.run(r.wg)
	}

	return nil
}

// Shuts down the radish executors and waits for them to finish processing all tasks.
func (r *Radish) Shutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.isRunning() {
		return
	}

	// Signal all executors to stop.
	for _, executor := range r.executors {
		executor.shutdown()
	}

	// Wait until all executors have finished.
	r.wg.Wait()

	r.executors = nil
	r.wg = nil
}

func (r *Radish) IsRunning() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isRunning()
}

func (r *Radish) isRunning() bool {
	return len(r.executors) > 0
}

//============================================================================
// Executor
//============================================================================

func (e *executor) run(wg *sync.WaitGroup) {
	// Create an unbuffered channel to signal the poll loop to stop.
	// This channel should block until a signal is received.
	stop := make(chan struct{})

	// Execute the poll loop in a goroutine.
	go func(stop <-chan struct{}, wg *sync.WaitGroup) {
		defer wg.Done()
		poll := time.NewTicker(e.conf.PollInterval)
		defer poll.Stop()

		// Poll for new tasks to execute
		for {
			select {
			case <-stop:
				return
			case <-poll.C:
				e.execute()
			}
		}
	}(stop, wg)

	// Store the stop channel for the executor to use to signal the poll loop to stop.
	e.stop = stop
}

func (e *executor) shutdown() {
	// Close the stop channel to signal the poll loop to stop and immediately return.
	// so that the executor is not blocking sending signals to the other executors.
	close(e.stop)
}

func (e *executor) execute() {
	// TODO: implement task execution logic here.
}
