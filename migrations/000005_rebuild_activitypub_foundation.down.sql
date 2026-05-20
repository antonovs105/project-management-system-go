DROP TABLE IF EXISTS actor_outbox_items CASCADE;
DROP TABLE IF EXISTS actor_inbox_items CASCADE;
DROP TABLE IF EXISTS project_invites CASCADE;
DROP TABLE IF EXISTS ap_activities CASCADE;
DROP TABLE IF EXISTS ap_objects CASCADE;
DROP TABLE IF EXISTS comments CASCADE;
DROP TABLE IF EXISTS ticket_assignees CASCADE;
DROP TABLE IF EXISTS ticket_links CASCADE;
DROP TABLE IF EXISTS tickets CASCADE;
DROP TABLE IF EXISTS actor_follows CASCADE;
DROP TABLE IF EXISTS project_members CASCADE;
DROP TABLE IF EXISTS projects CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS actor_keys CASCADE;
DROP TABLE IF EXISTS actors CASCADE;
DROP FUNCTION IF EXISTS set_updated_at() CASCADE;

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'worker',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    owner_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_owner FOREIGN KEY(owner_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE project_members (
    user_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,
    role VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, project_id),
    CONSTRAINT fk_user FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_project FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE tickets (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'new',
    priority VARCHAR(50) NOT NULL DEFAULT 'medium',
    project_id BIGINT NOT NULL,
    reporter_id BIGINT NOT NULL,
    assignee_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    type VARCHAR(20) DEFAULT 'task',
    parent_id BIGINT REFERENCES tickets(id),
    CONSTRAINT fk_project FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT fk_reporter FOREIGN KEY(reporter_id) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT fk_assignee FOREIGN KEY(assignee_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE labels (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE
);

CREATE TABLE ticket_labels (
    ticket_id BIGINT NOT NULL,
    label_id BIGINT NOT NULL,
    PRIMARY KEY (ticket_id, label_id),
    CONSTRAINT fk_ticket FOREIGN KEY(ticket_id) REFERENCES tickets(id) ON DELETE CASCADE,
    CONSTRAINT fk_label FOREIGN KEY(label_id) REFERENCES labels(id) ON DELETE CASCADE
);

CREATE TABLE ticket_links (
    id BIGSERIAL PRIMARY KEY,
    source_id BIGINT REFERENCES tickets(id) ON DELETE CASCADE,
    target_id BIGINT REFERENCES tickets(id) ON DELETE CASCADE,
    link_type VARCHAR(20),
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(source_id, target_id)
);

CREATE INDEX idx_tickets_parent_id ON tickets(parent_id);
CREATE INDEX idx_ticket_links_source_id ON ticket_links(source_id);
CREATE INDEX idx_ticket_links_target_id ON ticket_links(target_id);
