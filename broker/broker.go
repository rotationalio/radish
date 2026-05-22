package broker

import (
	"context"
	"fmt"
	"io"
	"time"

	"go.rtnl.ai/radish/broker/mock"
	"go.rtnl.ai/radish/broker/postgres"
	"go.rtnl.ai/radish/broker/sqlite"
	"go.rtnl.ai/radish/models"
	"go.rtnl.ai/x/dsn"
)

type Broker interface {
	io.Closer
	Info(ctx context.Context, id int64) (task *models.TaskMeta, err error)
	Enqueue(ctx context.Context, kind string, payload []byte) (id int64, err error)
	Schedule(ctx context.Context, kind string, payload []byte, executeAfter time.Time) (id int64, err error)
	Dequeue(ctx context.Context, ttl time.Duration) (task *models.TaskMeta, err error)
	Cancel(ctx context.Context, id int64) (err error)
	Fail(ctx context.Context, id int64, errors models.AttemptErrors) (err error)
	Retry(ctx context.Context, id int64, errors models.AttemptErrors, delay time.Duration) (err error)
	Success(ctx context.Context, id int64) (err error)
	Vacuum(ctx context.Context, retention time.Duration) (err error)
}

func Connect(databaseURL string) (b Broker, err error) {
	var uri *dsn.DSN
	if uri, err = dsn.Parse(databaseURL); err != nil {
		return nil, fmt.Errorf("could not parse broker connection string: %w", err)
	}

	switch uri.Provider {
	case dsn.Postgres:
		return postgres.Connect(uri)
	case dsn.SQLite3:
		return sqlite.Connect(uri)
	case "mock":
		return mock.Connect(uri)
	default:
		return nil, fmt.Errorf("unsupported broker provider: %s", uri.Provider)
	}
}
