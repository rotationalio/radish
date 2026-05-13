-- Dequeue the next task from the queue, skipping over locked rows.
-- Parameter: the visibility timeout for the task in seconds.
WITH next_task AS (
    SELECT id FROM radish_tasks
    WHERE
        status = 'pending' OR
        (status in ('running', 'scheduled', 'retry') AND visible_at <= NOW())
    ORDER BY created ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
UPDATE radish_tasks
SET status = 'running',
    attempts = attempts + 1,
    visible_at = NOW() + make_interval(secs := $1),
    last_attempt = NOW(),
    modified = NOW()
FROM next_task
WHERE radish_tasks.id = next_task.id
RETURNING radish_tasks.*;