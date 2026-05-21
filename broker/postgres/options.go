package postgres

import (
	"fmt"
	"strconv"
	"time"

	"go.rtnl.ai/x/dsn"
)

type PostgresOptions struct {
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

var defaultOptions = map[string]string{
	"sslmode":                   "prefer",
	"connect_timeout":           "60",
	"fallback_application_name": "radish",
}

func ConnectionOptions(dsn dsn.DSN) (connStr string, opts *PostgresOptions, err error) {
	// Make a copy of the database URL to avoid modifying the original.
	if dsn.Options == nil {
		dsn.Options = make(map[string]string, 0)
	}

	// Add default parameter configuration if not provided.
	for key, value := range defaultOptions {
		if _, ok := dsn.Options[key]; !ok {
			dsn.Options[key] = value
		}
	}

	// Parse the options into a PostgresOptions struct.
	opts = &PostgresOptions{
		MaxIdleConns:    128,
		MaxOpenConns:    512,
		ConnMaxLifetime: 60 * time.Minute,
		ConnMaxIdleTime: 6 * time.Minute,
	}

	if val, ok := dsn.Options["max_idle_conns"]; ok {
		if opts.MaxIdleConns, err = strconv.Atoi(val); err != nil {
			return "", nil, fmt.Errorf("could not parse max idle conns: %w", err)
		}
		delete(dsn.Options, "max_idle_conns")
	}

	if val, ok := dsn.Options["max_open_conns"]; ok {
		if opts.MaxOpenConns, err = strconv.Atoi(val); err != nil {
			return "", nil, fmt.Errorf("could not parse max open conns: %w", err)
		}
		delete(dsn.Options, "max_open_conns")
	}

	if val, ok := dsn.Options["conn_max_lifetime"]; ok {
		if opts.ConnMaxLifetime, err = time.ParseDuration(val); err != nil {
			return "", nil, fmt.Errorf("could not parse conn max lifetime: %w", err)
		}
		delete(dsn.Options, "conn_max_lifetime")
	}

	if val, ok := dsn.Options["conn_max_idle_time"]; ok {
		if opts.ConnMaxIdleTime, err = time.ParseDuration(val); err != nil {
			return "", nil, fmt.Errorf("could not parse conn max idle time: %w", err)
		}
		delete(dsn.Options, "conn_max_idle_time")
	}

	return dsn.String(), opts, nil
}
