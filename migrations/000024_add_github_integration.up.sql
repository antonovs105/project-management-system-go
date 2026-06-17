CREATE TABLE project_github_repositories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    owner TEXT NOT NULL,
    name TEXT NOT NULL,
    full_name TEXT NOT NULL,
    html_url TEXT NOT NULL DEFAULT '',
    default_branch TEXT NOT NULL DEFAULT '',
    last_synced_at TIMESTAMPTZ,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (owner <> ''),
    CHECK (name <> ''),
    UNIQUE (project_id, owner, name)
);

CREATE INDEX idx_project_github_repositories_project_id
    ON project_github_repositories(project_id);

CREATE TABLE github_commits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES project_github_repositories(id) ON DELETE CASCADE,
    sha TEXT NOT NULL,
    short_sha TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    author_name TEXT NOT NULL DEFAULT '',
    author_email TEXT NOT NULL DEFAULT '',
    authored_at TIMESTAMPTZ,
    html_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (sha <> ''),
    UNIQUE (repository_id, sha)
);

CREATE INDEX idx_github_commits_repository_id
    ON github_commits(repository_id);

CREATE INDEX idx_github_commits_authored_at
    ON github_commits(authored_at DESC);

CREATE TABLE github_commit_ticket_links (
    commit_id UUID NOT NULL REFERENCES github_commits(id) ON DELETE CASCADE,
    ticket_id UUID NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    link_source TEXT NOT NULL DEFAULT 'message'
        CHECK (link_source IN ('message', 'manual')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (commit_id, ticket_id)
);

CREATE INDEX idx_github_commit_ticket_links_ticket_id
    ON github_commit_ticket_links(ticket_id);
