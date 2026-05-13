-- Cleanup any completed tasks that are older than the retention period.
-- Parameter: the retention period in seconds.
DELETE FROM radish_tasks WHERE
    status IN ('succeeded', 'failed', 'cancelled') AND
    finished < NOW() - make_interval(secs := $1);