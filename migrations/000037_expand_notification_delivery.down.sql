DROP TABLE IF EXISTS notification_preferences;
DROP INDEX IF EXISTS idx_notifications_user_type_dedupe;

DELETE FROM notifications WHERE type <> 'ticket.assigned';

ALTER TABLE notifications
    DROP COLUMN IF EXISTS dedupe_key,
    DROP CONSTRAINT notifications_type_check,
    ADD CONSTRAINT notifications_type_check CHECK (type IN ('ticket.assigned'));
