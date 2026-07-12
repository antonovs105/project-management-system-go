ALTER TABLE notifications
    DROP CONSTRAINT notifications_type_check;

ALTER TABLE notifications
    ADD CONSTRAINT notifications_type_check CHECK (type IN (
        'ticket.assigned',
        'ticket.status_changed',
        'ticket.due_soon',
        'ticket.overdue',
        'comment.created',
        'comment.mentioned',
        'project.invited',
        'project.role_changed',
        'federation.delivery_failed',
        'security.event'
    )),
    ADD COLUMN dedupe_key TEXT;

CREATE UNIQUE INDEX idx_notifications_user_type_dedupe
    ON notifications (user_id, type, dedupe_key)
    WHERE dedupe_key IS NOT NULL;

CREATE TABLE notification_preferences (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN (
        'ticket.assigned',
        'ticket.status_changed',
        'ticket.due_soon',
        'ticket.overdue',
        'comment.created',
        'comment.mentioned',
        'project.invited',
        'project.role_changed',
        'federation.delivery_failed',
        'security.event'
    )),
    in_app_enabled BOOLEAN NOT NULL DEFAULT true,
    email_enabled BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, type)
);
