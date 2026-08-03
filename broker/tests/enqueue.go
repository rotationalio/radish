package tests

import (
	"context"
	"math/rand/v2"
	"time"

	"go.rtnl.ai/radish/broker"
	"go.rtnl.ai/radish/models"
)

type EnqueueConfig struct {
	Kind       string    // the kind to enqueue
	NPending   int       // the number of tasks to enqueue
	NRetry     int       // the number of tasks to mark as retryable
	NSuccess   int       // the number of tasks to mark as succeeded
	NFailed    int       // the number of tasks to mark as failed
	NScheduled int       // the number of tasks to schedule
	After      time.Time // the time to schedule the task after
	NCancelled int       // the number of scheduled tasks to mark as cancelled

	// The tasks that were enqueued
	tasks []int64
}

func (c *EnqueueConfig) Total() int64 {
	return int64(len(c.tasks))
}

func (c *EnqueueConfig) Enqueue(ctx context.Context, b broker.Broker) (err error) {
	p, r, s, f, sc, ca := c.NPending, c.NRetry, c.NSuccess, c.NFailed, c.NScheduled, c.NCancelled
	for p+r+s+f+sc+ca != 0 {
		if p > 0 {
			var id int64
			if id, err = b.Enqueue(ctx, c.Kind, []byte(testPayload), nil); err != nil {
				return err
			}

			c.tasks = append(c.tasks, id)
			p--
		}

		if r > 0 {
			var id int64
			if id, err = b.Enqueue(ctx, c.Kind, []byte(testPayload), nil); err != nil {
				return err
			}

			if err = b.Retry(ctx, id, models.AttemptErrors{{Attempt: 1, Error: "test", Timestamp: time.Now()}}, 0); err != nil {
				return err
			}

			c.tasks = append(c.tasks, id)
			r--
		}

		if s > 0 {
			var id int64
			if id, err = b.Enqueue(ctx, c.Kind, []byte(testPayload), nil); err != nil {
				return err
			}

			if err = b.Success(ctx, id); err != nil {
				return err
			}

			c.tasks = append(c.tasks, id)
			s--
		}

		if f > 0 {
			var id int64
			if id, err = b.Enqueue(ctx, c.Kind, []byte(testPayload), nil); err != nil {
				return err
			}

			if err = b.Fail(ctx, id, models.AttemptErrors{{Attempt: 1, Error: "test", Timestamp: time.Now()}}); err != nil {
				return err
			}

			c.tasks = append(c.tasks, id)
			f--
		}

		if sc > 0 {
			var id int64
			if id, err = b.Schedule(ctx, c.Kind, []byte(testPayload), c.After.Add(randDelay()), nil); err != nil {
				return err
			}

			c.tasks = append(c.tasks, id)
			sc--
		}

		if ca > 0 {
			var id int64
			if id, err = b.Schedule(ctx, c.Kind, []byte(testPayload), c.After.Add(randDelay()), nil); err != nil {
				return err
			}

			if err = b.Cancel(ctx, id); err != nil {
				return err
			}

			c.tasks = append(c.tasks, id)
			ca--
		}
	}

	return nil
}

func (c *EnqueueConfig) QueueSize() int64 {
	return int64(c.NPending + c.NRetry + c.NScheduled)
}

func randDelay() time.Duration {
	return time.Duration(rand.IntN(10000)) * time.Millisecond
}
