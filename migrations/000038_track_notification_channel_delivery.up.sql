ALTER TABLE notifications
    ADD COLUMN in_app_visible BOOLEAN NOT NULL DEFAULT true;

CREATE INDEX idx_notifications_user_visible_created
    ON notifications (user_id, created_at DESC, id DESC)
    WHERE in_app_visible = true;
