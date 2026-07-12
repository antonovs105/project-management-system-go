CREATE TABLE project_webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    target_url TEXT NOT NULL,
    secret_ciphertext TEXT NOT NULL,
    events TEXT[] NOT NULL CHECK (
        cardinality(events) > 0
        AND events <@ ARRAY[
            'project.created', 'project.updated', 'project.archived', 'project.restored',
            'ticket.created', 'ticket.updated', 'ticket.archived', 'ticket.restored'
        ]::TEXT[]
    ),
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_project_webhooks_project ON project_webhooks (project_id, created_at DESC);

CREATE TABLE project_webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL REFERENCES project_webhooks(id) ON DELETE CASCADE,
    activity_event_id UUID NOT NULL REFERENCES project_activity_events(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'delivered', 'failed', 'dead')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 8 CHECK (max_attempts > 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT NOT NULL DEFAULT '',
    last_status_code INTEGER,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (webhook_id, activity_event_id)
);

CREATE INDEX idx_project_webhook_deliveries_pending
    ON project_webhook_deliveries (next_attempt_at, created_at)
    WHERE status IN ('pending', 'failed');
