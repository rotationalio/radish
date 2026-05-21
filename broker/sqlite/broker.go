package sqlite

import (
	"context"
	"time"

	"go.rtnl.ai/radish/models"
	"go.rtnl.ai/x/dsn"
)

type Broker struct{}

func Connect(uri *dsn.DSN) (b *Broker, err error) {
	return &Broker{}, nil
}

func (b *Broker) Close() error {
	return nil
}

func (b *Broker) Info(ctx context.Context, id int64) (task *models.TaskMeta, err error) {
	return nil, nil
}

func (b *Broker) Enqueue(ctx context.Context, kind string, payload []byte) (id int64, err error) {
	return 0, nil
}

func (b *Broker) Schedule(ctx context.Context, kind string, payload []byte, executeAfter time.Time) (id int64, err error) {
	return 0, nil
}

func (b *Broker) Dequeue(ctx context.Context, ttl time.Duration) (task *models.TaskMeta, err error) {
	return nil, nil
}

func (b *Broker) Cancel(ctx context.Context, id int64) (err error) {
	return nil
}

func (b *Broker) Fail(ctx context.Context, id int64, errors models.AttemptErrors) (err error) {
	return nil
}

func (b *Broker) Retry(ctx context.Context, id int64, errors models.AttemptErrors, delay time.Duration) (err error) {
	return nil
}

func (b *Broker) Success(ctx context.Context, id int64) (err error) {
	return nil
}

func (b *Broker) Vacuum(ctx context.Context, retention time.Duration) (err error) {
	return nil
}
