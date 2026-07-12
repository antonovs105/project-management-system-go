ALTER TABLE tickets ADD COLUMN due_date TIMESTAMPTZ;
CREATE INDEX idx_tickets_project_due_date
    ON tickets (project_id, due_date)
    WHERE due_date IS NOT NULL AND is_resolved = false;
