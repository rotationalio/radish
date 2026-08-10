package models

import (
	"database/sql"
	"time"

	"go.rtnl.ai/radish/status"
)

type QueueStatus struct {
	Statuses       map[status.Status]int64
	Kinds          map[string]int64
	Awaiting       int64
	Completed      int64
	Earliest       sql.NullTime
	Latest         sql.NullTime
	ScheduledUntil sql.NullTime
}

func (q *QueueStatus) ScanStatuses(rows *sql.Rows) error {
	defer rows.Close()
	q.Statuses = make(map[status.Status]int64, 7)

	for rows.Next() {
		var state status.Status
		var count int64
		if err := rows.Scan(&state, &count); err != nil {
			return err
		}

		q.Statuses[state] = count
		switch {
		case state <= status.Running:
			q.Awaiting += count
		case state > status.Running:
			q.Completed += count
		}
	}
	return rows.Err()
}

func (q *QueueStatus) ScanKinds(rows *sql.Rows) error {
	defer rows.Close()
	q.Kinds = make(map[string]int64)

	for rows.Next() {
		var kind string
		var count int64
		if err := rows.Scan(&kind, &count); err != nil {
			return err
		}
		q.Kinds[kind] = count
	}
	return rows.Err()
}

func (q *QueueStatus) ScanTimes(s Scanner) error {
	return s.Scan(
		&q.Earliest,
		&q.Latest,
		&q.ScheduledUntil,
	)
}

type Series []*Period

type Period struct {
	Timestamp time.Time
	Tasks     int64
}

func (p *Period) Scan(s Scanner) error {
	return s.Scan(
		&p.Timestamp,
		&p.Tasks,
	)
}
