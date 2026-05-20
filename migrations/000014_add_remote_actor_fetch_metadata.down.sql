DROP INDEX IF EXISTS idx_actors_remote_last_fetched_at;

ALTER TABLE actors
DROP COLUMN IF EXISTS fetch_error,
DROP COLUMN IF EXISTS last_fetched_at;
