#!/bin/bash

export DATABASE_URL=postgres://radish@localhost:5432/radish_test?sslmode=disable
export RADISH_MANAGED_DB=false
export RADISH_NUM_WORKERS=8
export RADISH_TASK_RETRIES=3
export RADISH_TASK_TIMEOUT=120s
export RADISH_POLL_INTERVAL=15s
export RADISH_POLL_JITTER=1250ms
export RADISH_RETENTION=5m
export RADISH_VACUUM_INTERVAL=2m
export RADISH_BACKOFF_POLICY=exponential
export RADISH_BACKOFF_DELAY=16s
export RADISH_BACKOFF_FACTOR=2.0
export RADISH_BACKOFF_JITTER=true
export RADISH_BACKOFF_SIGMA=1500ms

if [ ! -f turnip ]; then
    go build -o turnip ../cmd/turnip/
fi

until false; do
    # ./turnip -path simulators.json
    ./turnip -radish-only

    # Capture the exit code of the turnip command.
    status=$?

    if [ $status -eq 9 ]; then
        echo "turnip crashed, sleeping for 16 seconds and restarting"
        sleep 16
    else
        exit $status
    fi
done
