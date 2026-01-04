ALTER TABLE tickets ADD COLUMN type VARCHAR(20) DEFAULT 'task';
ALTER TABLE tickets ADD COLUMN parent_id BIGINT REFERENCES tickets(id);

CREATE TABLE ticket_links (
    id BIGSERIAL PRIMARY KEY,
    source_id BIGINT REFERENCES tickets(id) ON DELETE CASCADE,
    target_id BIGINT REFERENCES tickets(id) ON DELETE CASCADE,
    link_type VARCHAR(20),
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(source_id, target_id)
);

CREATE INDEX idx_tickets_parent_id ON tickets(parent_id);
CREATE INDEX idx_ticket_links_source_id ON ticket_links(source_id);
CREATE INDEX idx_ticket_links_target_id ON ticket_links(target_id);
