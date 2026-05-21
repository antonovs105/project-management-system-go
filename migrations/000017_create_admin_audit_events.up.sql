CREATE TABLE admin_audit_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL CHECK (
        action IN (
            'user.role_updated',
            'federation.domain_blocked',
            'federation.domain_unblocked',
            'federation.delivery_retried'
        )
    ),
    target_type TEXT NOT NULL CHECK (
        target_type IN ('user', 'federation_domain', 'federation_delivery')
    ),
    target_id TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_admin_audit_events_created_at
    ON admin_audit_events(created_at DESC);

CREATE INDEX idx_admin_audit_events_actor_user_id
    ON admin_audit_events(actor_user_id);

CREATE INDEX idx_admin_audit_events_action
    ON admin_audit_events(action);

CREATE INDEX idx_admin_audit_events_target
    ON admin_audit_events(target_type, target_id);
