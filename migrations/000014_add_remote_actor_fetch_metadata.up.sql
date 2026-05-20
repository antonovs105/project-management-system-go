ALTER TABLE actors
ADD COLUMN last_fetched_at TIMESTAMPTZ,
ADD COLUMN fetch_error TEXT;

CREATE INDEX idx_actors_remote_last_fetched_at
    ON actors (last_fetched_at)
    WHERE is_local = false;
