DROP INDEX IF EXISTS idx_notifications_user_visible_created;

ALTER TABLE notifications
    DROP COLUMN IF EXISTS in_app_visible;
