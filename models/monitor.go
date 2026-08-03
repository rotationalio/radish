package models

import (
	"time"

	"go.rtnl.ai/radish/status"
)

type QueueStatus struct {
	Statuses       map[status.Status]int64
	Kinds          map[string]int64
	Awaiting       int64
	Completed      int64
	Earliest       time.Time
	Latest         time.Time
	ScheduledUntil time.Time
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
