package cursor

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lib/pq"
	"go.rtnl.ai/radish/status"
	"go.rtnl.ai/x/dsn"
)

type Filter struct {
	kinds  []string
	states []status.Status
	before time.Time
	after  time.Time
	params []any
}

type HasKind interface {
	Kind() string
}

type HasKindAliases interface {
	KindAliases() []string
}

// Where creates a new filter with no initial conditions.
func Where() *Filter {
	return &Filter{}
}

func (f *Filter) Clause(dialect string) string {
	// Set up the clause builder and parameters slice.
	f.params = make([]any, 0, 4)
	sb := strings.Builder{}
	where := false

	// Filter on kinds if specified.
	if len(f.kinds) > 0 {
		// First condition, no need to check if where is true.
		sb.WriteString(" WHERE ")
		where = true

		switch dialect {
		case dsn.Postgres:
			fmt.Fprintf(&sb, "kind = ANY($%d)", len(f.params)+1)
			f.params = append(f.params, pq.Array(f.kinds))
		case dsn.SQLite3:
			placeholders := make([]string, 0, len(f.kinds))
			for i, kind := range f.kinds {
				placeholder := fmt.Sprintf("kind%d", i+1)
				placeholders = append(placeholders, ":"+placeholder)
				f.params = append(f.params, sql.Named(placeholder, kind))
			}

			fmt.Fprintf(&sb, "kind IN (%s)", strings.Join(placeholders, ","))
		default:
			sb.WriteString("kind IN ?")
			f.params = append(f.params, f.kinds)
		}
	}

	// Filter on states if specified.
	if len(f.states) > 0 {
		if !where {
			sb.WriteString(" WHERE ")
			where = true
		} else {
			sb.WriteString(" AND ")
		}

		switch dialect {
		case dsn.Postgres:
			fmt.Fprintf(&sb, "status = ANY($%d)", len(f.params)+1)
			f.params = append(f.params, pq.Array(f.states))
		case dsn.SQLite3:
			placeholders := make([]string, 0, len(f.states))
			for i, state := range f.states {
				placeholder := fmt.Sprintf("state%d", i+1)
				placeholders = append(placeholders, ":"+placeholder)
				f.params = append(f.params, sql.Named(placeholder, state))
			}
			fmt.Fprintf(&sb, "status IN (%s)", strings.Join(placeholders, ","))
		default:
			sb.WriteString("status IN ?")
			f.params = append(f.params, f.states)
		}
	}

	// Filter on visible_at before if specified.
	if !f.before.IsZero() {
		if !where {
			sb.WriteString(" WHERE ")
			where = true
		} else {
			sb.WriteString(" AND ")
		}

		switch dialect {
		case dsn.Postgres:
			fmt.Fprintf(&sb, "visible_at < $%d", len(f.params)+1)
			f.params = append(f.params, f.before)
		case dsn.SQLite3:
			fmt.Fprintf(&sb, "visible_at < :before")
			f.params = append(f.params, sql.Named("before", f.before))
		default:
			sb.WriteString("visible_at < ?")
			f.params = append(f.params, f.before)
		}
	}

	// Filter on visible_at after if specified.
	// Last condition, no need to set where to true.
	if !f.after.IsZero() {
		if !where {
			sb.WriteString(" WHERE ")
		} else {
			sb.WriteString(" AND ")
		}

		switch dialect {
		case dsn.Postgres:
			fmt.Fprintf(&sb, "visible_at >= $%d", len(f.params)+1)
			f.params = append(f.params, f.after)
		case dsn.SQLite3:
			fmt.Fprintf(&sb, "visible_at >= :after")
			f.params = append(f.params, sql.Named("after", f.after))
		default:
			sb.WriteString("visible_at >= ?")
			f.params = append(f.params, f.after)
		}
	}

	return sb.String()
}

func (f *Filter) Params() []any {
	return f.params
}

// Specify the kinds of tasks to filter by, this will replace any existing kinds on
// the filter and return the current filter, modified to only include the new kinds.
// You can specify the kinds as either a string or as a Task type. If its a Task type,
// then all the kinds and kind aliases are included. If a kind type is specified that
// is unknown then it is ignored without error.
func (f *Filter) Kinds(kinds ...any) *Filter {
	f.kinds = make([]string, 0, len(kinds))
	for _, kind := range kinds {
		switch kind := kind.(type) {
		case string:
			f.kinds = append(f.kinds, kind)
		case HasKind:
			f.kinds = append(f.kinds, kind.Kind())
			if aliases, ok := kind.(HasKindAliases); ok {
				f.kinds = append(f.kinds, aliases.KindAliases()...)
			}
		case HasKindAliases:
			f.kinds = append(f.kinds, kind.KindAliases()...)
		case fmt.Stringer:
			f.kinds = append(f.kinds, kind.String())
		}
	}

	// Deduplicate the kinds.
	slices.Sort(f.kinds)
	f.kinds = slices.Compact(f.kinds)
	return f
}

// Specify the states of tasks to filter by, this will replace any existing states on
// the filter and return the current filter, modified to only include the new states.
func (f *Filter) States(states ...status.Status) *Filter {
	f.states = make([]status.Status, 0, len(states))
	for _, state := range states {
		if state != status.Unknown {
			f.states = append(f.states, state)
		}
	}

	// Deduplicate the states.
	slices.Sort(f.states)
	f.states = slices.Compact(f.states)
	return f
}

// Completed will filter tasks that are in the succeeded, failed, or cancelled states.
func (f *Filter) Completed() *Filter {
	return f.States(status.Succeeded, status.Failed, status.Cancelled)
}

// Awaiting will filter tasks that are in the pending, retry, or scheduled states.
func (f *Filter) Awaiting() *Filter {
	return f.States(status.Pending, status.Retry, status.Scheduled)
}

// Before will filter tasks that are visible before the given time.
// NOTE: this uses the visible_at column, not the created column.
func (f *Filter) Before(before time.Time) *Filter {
	f.before = before
	return f
}

// After will filter tasks that are visible after the given time.
// NOTE: this uses the visible_at column, not the created column.
func (f *Filter) After(after time.Time) *Filter {
	f.after = after
	return f
}
