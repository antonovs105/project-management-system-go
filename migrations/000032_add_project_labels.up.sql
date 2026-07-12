CREATE TABLE project_labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(50) NOT NULL,
    color VARCHAR(7) NOT NULL CHECK (color ~ '^#[0-9A-Fa-f]{6}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

CREATE TABLE ticket_labels (
    ticket_id UUID NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    label_id UUID NOT NULL REFERENCES project_labels(id) ON DELETE CASCADE,
    PRIMARY KEY (ticket_id, label_id)
);

CREATE INDEX idx_project_labels_project_id ON project_labels(project_id);
CREATE INDEX idx_ticket_labels_label_id ON ticket_labels(label_id);
