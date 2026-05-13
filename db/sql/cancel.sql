-- Mark a task as cancelled with no additional errors.
UPDATE radish_tasks
SET status = 'cancelled',
    visible_at = NULL,
    finished = NOW(),
    modified = NOW()
WHERE id = $1