BEGIN;

-- Create the radish_status type if it does not exist.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'radish_status') THEN
        CREATE TYPE radish_status AS ENUM (
            'pending',
            'retry',
            'scheduled',
            'running',
            'succeeded',
            'failed',
            'cancelled',
        );
    END IF;
END $$;

-- Create the radish_tasks table if it does not exist.
CREATE TABLE IF NOT EXISTS radish_tasks (
    id              BIGSERIAL PRIMARY KEY,
    kind            VARCHAR(255) NOT NULL,
    status          radish_status NOT NULL DEFAULT 'pending',
    payload         JSONB NOT NULL DEFAULT '{}',
    attempts        SMALLINT NOT NULL DEFAULT 0,
    errors          JSONB NOT NULL DEFAULT '[]',
    visible_at      TIMESTAMPTZ DEFAULT NULL,
    last_attempt    TIMESTAMPTZ DEFAULT NULL,
    finished        TIMESTAMPTZ DEFAULT NULL,
    created         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modified        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMIT;