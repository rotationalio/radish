package cursor_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	. "go.rtnl.ai/radish/broker/cursor"
	"go.rtnl.ai/radish/status"
	"go.rtnl.ai/x/dsn"
)

func TestFilter(t *testing.T) {
	ts := time.Date(2026, 7, 30, 8, 32, 14, 345923, time.UTC)

	tests := []struct {
		name       string
		filter     *Filter
		dialect    string
		want       string
		wantParams []any
	}{
		{
			name:       "PostgreSQL/Kinds",
			filter:     Where().Kinds("test", "foo", "bar"),
			dialect:    dsn.Postgres,
			want:       " WHERE kind = ANY($1)",
			wantParams: []any{[]string{"bar", "foo", "test"}},
		},
		{
			name:       "SQLite3/Kinds",
			filter:     Where().Kinds("test", "foo", "bar"),
			dialect:    dsn.SQLite3,
			want:       " WHERE kind IN (:kind1,:kind2,:kind3)",
			wantParams: []any{sql.Named("kind1", "bar"), sql.Named("kind2", "foo"), sql.Named("kind3", "test")},
		},
		{
			name:       "Default/Kinds",
			filter:     Where().Kinds("test", "foo", "bar", "foo", "test"),
			dialect:    "",
			want:       " WHERE kind IN ?",
			wantParams: []any{[]string{"bar", "foo", "test"}},
		},
		{
			name:       "PostgreSQL/Completed",
			filter:     Where().Completed(),
			dialect:    dsn.Postgres,
			want:       " WHERE status = ANY($1)",
			wantParams: []any{[]status.Status{status.Succeeded, status.Failed, status.Cancelled}},
		},
		{
			name:       "SQLite3/Completed",
			filter:     Where().Completed(),
			dialect:    dsn.SQLite3,
			want:       " WHERE status IN (:state1,:state2,:state3)",
			wantParams: []any{sql.Named("state1", status.Succeeded), sql.Named("state2", status.Failed), sql.Named("state3", status.Cancelled)},
		},
		{
			name:       "Default/Completed",
			filter:     Where().Completed(),
			dialect:    "",
			want:       " WHERE status IN ?",
			wantParams: []any{[]status.Status{status.Succeeded, status.Failed, status.Cancelled}},
		},
		{
			name:       "PostgreSQL/Awaiting",
			filter:     Where().Awaiting(),
			dialect:    dsn.Postgres,
			want:       " WHERE status = ANY($1)",
			wantParams: []any{[]status.Status{status.Pending, status.Retry, status.Scheduled}},
		},
		{
			name:       "SQLite3/Awaiting",
			filter:     Where().Awaiting(),
			dialect:    dsn.SQLite3,
			want:       " WHERE status IN (:state1,:state2,:state3)",
			wantParams: []any{sql.Named("state1", status.Pending), sql.Named("state2", status.Retry), sql.Named("state3", status.Scheduled)},
		},
		{
			name:       "Default/Awaiting",
			filter:     Where().Awaiting(),
			dialect:    "",
			want:       " WHERE status IN ?",
			wantParams: []any{[]status.Status{status.Pending, status.Retry, status.Scheduled}},
		},
		{
			name:       "PostgreSQL/Before",
			filter:     Where().Before(ts),
			dialect:    dsn.Postgres,
			want:       " WHERE visible_at < $1",
			wantParams: []any{ts},
		},
		{
			name:       "SQLite3/Before",
			filter:     Where().Before(ts),
			dialect:    dsn.SQLite3,
			want:       " WHERE visible_at < :before",
			wantParams: []any{sql.Named("before", ts)},
		},
		{
			name:       "Default/Before",
			filter:     Where().Before(ts),
			dialect:    "",
			want:       " WHERE visible_at < ?",
			wantParams: []any{ts},
		},
		{
			name:       "PostgreSQL/After",
			filter:     Where().After(ts),
			dialect:    dsn.Postgres,
			want:       " WHERE visible_at >= $1",
			wantParams: []any{ts},
		},
		{
			name:       "SQLite3/After",
			filter:     Where().After(ts),
			dialect:    dsn.SQLite3,
			want:       " WHERE visible_at >= :after",
			wantParams: []any{sql.Named("after", ts)},
		},
		{
			name:       "Default/After",
			filter:     Where().After(ts),
			dialect:    "",
			want:       " WHERE visible_at >= ?",
			wantParams: []any{ts},
		},
		{
			name:       "PostgreSQL/BeforeAndAfter",
			filter:     Where().Before(ts).After(ts),
			dialect:    dsn.Postgres,
			want:       " WHERE visible_at < $1 AND visible_at >= $2",
			wantParams: []any{ts, ts},
		},
		{
			name:       "SQLite3/BeforeAndAfter",
			filter:     Where().Before(ts).After(ts),
			dialect:    dsn.SQLite3,
			want:       " WHERE visible_at < :before AND visible_at >= :after",
			wantParams: []any{sql.Named("before", ts), sql.Named("after", ts)},
		},
		{
			name:       "Default/BeforeAndAfter",
			filter:     Where().Before(ts).After(ts),
			dialect:    "",
			want:       " WHERE visible_at < ? AND visible_at >= ?",
			wantParams: []any{ts, ts},
		},
		{
			name:       "PostgreSQL/KindsAndCompleted",
			filter:     Where().Kinds("test", "foo", "bar").Completed(),
			dialect:    dsn.Postgres,
			want:       " WHERE kind = ANY($1) AND status = ANY($2)",
			wantParams: []any{[]string{"bar", "foo", "test"}, []status.Status{status.Succeeded, status.Failed, status.Cancelled}},
		},
		{
			name:       "SQLite3/KindsAndCompleted",
			filter:     Where().Kinds("test", "foo", "bar").Completed(),
			dialect:    dsn.SQLite3,
			want:       " WHERE kind IN (:kind1,:kind2,:kind3) AND status IN (:state1,:state2,:state3)",
			wantParams: []any{sql.Named("kind1", "bar"), sql.Named("kind2", "foo"), sql.Named("kind3", "test"), sql.Named("state1", status.Succeeded), sql.Named("state2", status.Failed), sql.Named("state3", status.Cancelled)},
		},
		{
			name:       "Default/KindsAndCompleted",
			filter:     Where().Kinds("test", "foo", "bar").Completed(),
			dialect:    "",
			want:       " WHERE kind IN ? AND status IN ?",
			wantParams: []any{[]string{"bar", "foo", "test"}, []status.Status{status.Succeeded, status.Failed, status.Cancelled}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.filter.Clause(test.dialect)
			require.Equal(t, test.want, got)
			require.Equal(t, test.wantParams, test.filter.Params())
		})
	}
}
