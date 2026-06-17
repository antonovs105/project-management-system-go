DROP INDEX IF EXISTS idx_project_github_repositories_remote;

ALTER TABLE project_github_repositories
    DROP COLUMN IF EXISTS last_webhook_at,
    DROP COLUMN IF EXISTS last_sync_error;
