package radish

import "github.com/pborman/uuid"

// Task specifies the interface for custom task types to be implemented.
// When registring a task with the radish server, it is important to note that the task
// methods may be called from multiple go routines; therefore the Handle, Success, and
// Failure methods must all be thread safe. You can treat the task similar to an
// http.Handler, constructing per-request handling functions on demand.
//
// TODO: require context and deadlines for task completion. Move ID to the context.
type Task interface {
	Name() string                                   // should return a unique name for the specified task
	Handle(id uuid.UUID, params []byte) error       // handle the task with the specified params in any serialization format
	Success(id uuid.UUID, params []byte)            // callback for when the task has successfully been completed without error
	Failure(id uuid.UUID, err error, params []byte) // callback for when the task could not be completed with the error
}

// Future represents an enqueued task and its serialized parameters
type Future struct {
	ID      uuid.UUID // Task ID
	Task    string    // Task type
	Params  []byte    // the serialized parameters of the future
	Success []byte    // the serialized parameters to pass to the success function
	Failure []byte    // the serialized parameters to pass to the failure function on error
}
