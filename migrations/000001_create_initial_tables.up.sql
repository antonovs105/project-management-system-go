CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'worker',
    full_name VARCHAR(255) NOT NULL,
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