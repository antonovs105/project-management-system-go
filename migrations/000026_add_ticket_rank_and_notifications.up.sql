ALTER TABLE tickets
    ADD COLUMN rank TEXT NOT NULL DEFAULT 'HZZZZZZZZZZZ';

WITH ranked AS (
    SELECT
        id,
        lpad((row_number() OVER (
            PARTITION BY project_id, status
            ORDER BY created_at ASC, id ASC
        ) * 1000000)::text, 12, '0') AS new_rank
    FROM tickets
)
UPDATE tickets
SET rank = ranked.new_rank
FROM ranked
WHERE ranked.id = tickets.id;

ALTER TABLE tickets
    ADD CONSTRAINT chk_tickets_rank_format CHECK (rank ~ '^[0-9A-Z]{12}$');

CREATE INDEX idx_tickets_project_status_rank
    ON tickets(project_id, status, rank, id);

CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    actor_id UUID REFERENCES actors(id) ON DELETE SET NULL,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    ticket_id UUID REFERENCES tickets(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('ticket.assigned')),
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notifications_user_created
    ON notifications(user_id, created_at DESC, id DESC);

CREATE INDEX idx_notifications_user_unread
    ON notifications(user_id, created_at DESC)
    WHERE read_at IS NULL;
