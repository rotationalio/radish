package radish

import "github.com/bbengfort/alia/out"

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
			} else {
				// Task success
				out.Debug("finished %s task %s", task.Task, task.ID)
				handler.Success(task.ID, task.Success)
			}
		}
	}
}
