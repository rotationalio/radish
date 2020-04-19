package radish_test

import (
	"sync"
	"sync/atomic"

	"github.com/pborman/uuid"
)

type testTask struct {
	wg        *sync.WaitGroup // concurrency management for tests
	name      string          // set a unique name for the test
	handled   int32           // number of times Handle was called
	successes int32           // number of times Success was called (calls wg.Done)
	failures  int32           // number of times Failure was called (calls wg.Done)
	onHandle  func(id uuid.UUID, params []byte) error
	onSuccess func(id uuid.UUID, params []byte)
	onFailure func(id uuid.UUID, err error, params []byte)
}

func (t *testTask) Name() string {
	if t.name != "" {
		return t.name
	}
	return "test"
}

func (t *testTask) Handle(id uuid.UUID, params []byte) error {
	atomic.AddInt32(&t.handled, 1)
	if t.onHandle != nil {
		return t.onHandle(id, params)
	}
	return nil
}

func (t *testTask) Success(id uuid.UUID, params []byte) {
	atomic.AddInt32(&t.successes, 1)
	if t.onSuccess != nil {
		t.onSuccess(id, params)
	}
	t.wg.Done()
}

func (t *testTask) Failure(id uuid.UUID, err error, params []byte) {
	atomic.AddInt32(&t.failures, 1)
	if t.onFailure != nil {
		t.onFailure(id, err, params)
	}
	t.wg.Done()
}
