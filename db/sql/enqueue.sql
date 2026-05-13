-- Enqueue a task with the given kind and payload.
-- Parameter: the kind of task to enqueue and its JSON encoded payload.
INSERT INTO radish_tasks (kind, payload) VALUES ($1, $2) RETURNING id;