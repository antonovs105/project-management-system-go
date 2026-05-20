ALTER TABLE actors
DROP COLUMN IF EXISTS fetch_error_at;

DROP INDEX IF EXISTS idx_activity_deliveries_last_failure_kind;

ALTER TABLE activity_deliveries
DROP COLUMN IF EXISTS last_status_code,
DROP COLUMN IF EXISTS last_failure_kind,
DROP COLUMN IF EXISTS last_attempt_at;
