DROP INDEX IF EXISTS idx_notifications_user_unread;
DROP INDEX IF EXISTS idx_notifications_user_created;
DROP TABLE IF EXISTS notifications;

DROP INDEX IF EXISTS idx_tickets_project_status_rank;

ALTER TABLE tickets
    DROP CONSTRAINT IF EXISTS chk_tickets_rank_format;

ALTER TABLE tickets
    DROP COLUMN IF EXISTS rank;
