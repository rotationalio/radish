-- Mark a task as failed with the given errors.
UPDATE radish_tasks
SET status = 'failed',
    errors = $2,
    visible_at = NULL,
    finished = NOW(),
    modified = NOW()
WHERE id = $1