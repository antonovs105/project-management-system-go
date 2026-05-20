ALTER TABLE activity_deliveries
ADD COLUMN last_attempt_at TIMESTAMPTZ,
ADD COLUMN last_failure_kind TEXT NOT NULL DEFAULT '',
ADD COLUMN last_status_code INTEGER;

CREATE INDEX idx_activity_deliveries_last_failure_kind
    ON activity_deliveries(last_failure_kind)
    WHERE last_failure_kind <> '';

ALTER TABLE actors
ADD COLUMN fetch_error_at TIMESTAMPTZ;
