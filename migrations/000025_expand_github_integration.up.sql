ALTER TABLE project_github_repositories
    ADD COLUMN last_sync_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN last_webhook_at TIMESTAMPTZ;

CREATE INDEX idx_project_github_repositories_remote
    ON project_github_repositories(owner, name);
