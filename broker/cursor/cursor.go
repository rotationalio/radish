package cursor

import (
	"database/sql"

	"go.rtnl.ai/radish/models"
)

// Cursor is a wrapper for *sql.Rows that allows us to iterate over the rows and access
// the task metadata. This interface allows us to stream the results of a query without
// materializing them all into a slice in memory.
type Cursor struct {
	tx    *sql.Tx
	rows  *sql.Rows
	count int64
}

func New(tx *sql.Tx, rows *sql.Rows, count int64) *Cursor {
	return &Cursor{tx: tx, rows: rows, count: count}
}

func (c *Cursor) Count() int64 {
	return c.count
}

func (c *Cursor) Task() (task *models.TaskMeta, err error) {
	task = new(models.TaskMeta)
	if err = task.Scan(c.rows); err != nil {
		return task, err
	}
	return task, nil
}

func (c *Cursor) List() (tasks []*models.TaskMeta, err error) {
	tasks = make([]*models.TaskMeta, 0, c.count)
	for c.Next() {
		var task *models.TaskMeta
		if task, err = c.Task(); err != nil {
			return tasks, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (c *Cursor) Next() bool {
	return c.rows.Next()
}

func (c *Cursor) Err() error {
	return c.rows.Err()
}

func (c *Cursor) Close() error {
	err := c.rows.Close()
	c.tx.Rollback()
	return err
}
