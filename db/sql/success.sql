-- Mark a task as succeeded with no additional errors.
UPDATE radish_tasks
SET status = 'succeeded',
    visible_at = NULL,
    finished = NOW(),
    modified = NOW()
WHERE id = $1;