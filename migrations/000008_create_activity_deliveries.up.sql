CREATE TABLE activity_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id UUID NOT NULL REFERENCES ap_activities(id) ON DELETE CASCADE,
    activity_ap_id TEXT NOT NULL,
    actor_id UUID NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    target_inbox_url TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'processing', 'delivered', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 10 CHECK (max_attempts > 0),
    next_attempt_at TIMESTAMPTZ,
    last_error TEXT,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(activity_id, target_inbox_url)
);

CREATE INDEX idx_activity_deliveries_state_next_attempt
    ON activity_deliveries(state, next_attempt_at);

CREATE INDEX idx_activity_deliveries_activity_id
    ON activity_deliveries(activity_id);

CREATE TRIGGER trg_activity_deliveries_updated_at BEFORE UPDATE ON activity_deliveries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
