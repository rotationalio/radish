package radish

import (
	"time"

	"github.com/kansaslabs/x/out"
)

type worker struct {
	parent *Radish   // the parent of the worker that has the tasks queue and the handlers
	stop   chan bool // gracefully stop the worker, do not process any more tasks
}

func (w *worker) run() {
taskloop:
	for {
		select {
		case <-w.stop:
			return
		case task := <-w.parent.tasks:

			// Update the queue size and percent full
			pmQueueSize.Set(float64(len(w.parent.tasks)))
			pmPercentFull.Set(float64(len(w.parent.tasks)) / float64(w.parent.config.QueueSize) * 100)

			start := time.Now()

			handler, err := w.parent.Handler(task.Task)
			if err != nil {
				// Unregistered task
				out.Warn("cannot handle unregistered task %q -- not processing %s", task.Task, task.ID)
				continue taskloop
			}

			// Handle the task
			if err := handler.Handle(task.ID, task.Params); err != nil {
				// Task failure
				out.Caution(err.Error())
				handler.Failure(task.ID, err, task.Failure)

				// Compute latency in milliseconds
				latency := float64(time.Since(start)/1000) / 1000.0
				pmTaskLatency.WithLabelValues(task.Task, "failed").Observe(latency)

				// Update prometheus metrics with failed task
				pmTasksFailed.WithLabelValues(task.Task).Inc()
			} else {
				// Task success
				out.Debug("finished %s task %s", task.Task, task.ID)
				handler.Success(task.ID, task.Success)

				// Compute latency in milliseconds
				latency := float64(time.Since(start)/1000) / 1000.0
				pmTaskLatency.WithLabelValues(task.Task, "succeeded").Observe(latency)

				// Update prometheus metrics with succeeded task
				pmTasksSucceeded.WithLabelValues(task.Task).Inc()
			}

		}
	}
}
