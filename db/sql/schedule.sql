-- Schedule a task with the given kind and payload to be executed later.
-- Parameter: the kind of task to schedule and its JSON encoded payload and the timestamp when it should be executed after.
INSERT INTO radish_tasks (kind, status, payload, visible_at)
    VALUES ($1, 'scheduled', $2, $3) RETURNING id;