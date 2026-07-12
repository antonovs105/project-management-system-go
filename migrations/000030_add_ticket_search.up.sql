ALTER TABLE tickets
    ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (
        to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(description, ''))
    ) STORED;

CREATE INDEX idx_tickets_search_vector
    ON tickets USING GIN (search_vector);
