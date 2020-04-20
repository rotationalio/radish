package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/kansaslabs/x/out"
	"github.com/pborman/uuid"
)

// Turnip is a probabilistic mock task that sleeps for a random duration and which may
// error with a specific probability. It does not accept any params in its handle method
// and its callbacks are no-ops. This task is primarily designed for testing the Radish
// task queue and benchmarking it.
type Turnip struct {
	name     string
	minDelay time.Duration
	maxDelay time.Duration
	errProb  float64
}

// Name returns the name of the task
func (t *Turnip) Name() string {
	if t.name != "" {
		return t.name
	}
	return "turnip"
}

// Handle sleeps for a random amount of time and returns an error with some probability.
func (t *Turnip) Handle(id uuid.UUID, params []byte) (err error) {
	delay := time.Duration(rand.Int63n(int64(t.maxDelay))) + t.minDelay
	out.Info("sleeping for %s", delay)
	time.Sleep(delay)

	if rand.Float64() <= t.errProb {
		return fmt.Errorf("%s errored after %s sleep with %0.2f probability", id, delay, t.errProb)
	}
	return nil
}

// Success callback is a no-op
func (t *Turnip) Success(id uuid.UUID, params []byte) {}

// Failure callback is a no-op
func (t *Turnip) Failure(id uuid.UUID, err error, params []byte) {}
