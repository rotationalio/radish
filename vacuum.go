package radish

import (
	"context"
	"sync"
	"time"

	"go.rtnl.ai/radish/broker"
	"go.rtnl.ai/x/rlog"
)

type vacuum struct {
	conf   *Config
	broker broker.Broker
	stop   chan<- struct{}
}

func (v *vacuum) run(wg *sync.WaitGroup) {
	// Create an unbuffered channel to signal the vacuum loop to stop.
	stop := make(chan struct{})

	// Execute the vacuum loop in a goroutine.
	go func(stop <-chan struct{}, wg *sync.WaitGroup) {
		defer wg.Done()

		// Vacuum the database at startup.
		if err := v.vacuum(); err != nil {
			rlog.Error("unable to vacuum tasks", "error", err)
		} else {
			rlog.Debug("radish tasks vacuumed", "retention", v.conf.Retention)
		}

		// Create ticker to vacuum at the interval.
		poll := time.NewTicker(v.conf.VacuumInterval)
		defer poll.Stop()

		for {
			select {
			case <-stop:
				return
			case <-poll.C:
				if err := v.vacuum(); err != nil {
					rlog.Error("unable to vacuum tasks", "error", err)
				} else {
					rlog.Debug("radish tasks vacuumed", "retention", v.conf.Retention)
				}
			}
		}
	}(stop, wg)

	v.stop = stop
}

func (v *vacuum) vacuum() error {
	ctx, cancel := context.WithTimeout(context.Background(), v.conf.VacuumInterval)
	defer cancel()

	return v.broker.Vacuum(ctx, v.conf.Retention)
}

func (v *vacuum) shutdown() {
	// Close the stop channel to signal the vacuum loop to stop and immediately return.
	// so that the vacuum loop is not blocking sending signals to the other vacuum loops.
	close(v.stop)
}
