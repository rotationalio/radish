-- Mark a task as pending with the given errors and retry the task.
-- Parameters: the task id, the errors to add to the task, and the delay before the next retry in seconds.
UPDATE radish_tasks
SET status = 'retry',
    errors = $2
    visible_at = NOW() + make_interval(secs := $3),
    modified = NOW()
WHERE id = $1;