CREATE EXTENSION IF NOT EXISTS pgcrypto;

DROP TABLE IF EXISTS actor_outbox_items CASCADE;
DROP TABLE IF EXISTS actor_inbox_items CASCADE;
DROP TABLE IF EXISTS ap_activities CASCADE;
DROP TABLE IF EXISTS ap_objects CASCADE;
DROP TABLE IF EXISTS comments CASCADE;
DROP TABLE IF EXISTS ticket_assignees CASCADE;
DROP TABLE IF EXISTS ticket_links CASCADE;
DROP TABLE IF EXISTS tickets CASCADE;
DROP TABLE IF EXISTS project_invites CASCADE;
DROP TABLE IF EXISTS actor_follows CASCADE;
DROP TABLE IF EXISTS project_members CASCADE;
DROP TABLE IF EXISTS projects CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS actor_keys CASCADE;
DROP TABLE IF EXISTS actors CASCADE;
DROP TABLE IF EXISTS ticket_labels CASCADE;
DROP TABLE IF EXISTS labels CASCADE;

CREATE TABLE actors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ap_id TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL CHECK (type IN ('Person', 'Group')),
    preferred_username TEXT NOT NULL,
    handle TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    inbox_url TEXT NOT NULL,
    outbox_url TEXT NOT NULL,
    followers_url TEXT NOT NULL,
    following_url TEXT,
    is_local BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_actors_local_preferred_username
    ON actors (lower(preferred_username))
    WHERE is_local;

CREATE TABLE actor_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id UUID NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    key_id TEXT NOT NULL UNIQUE,
    algorithm TEXT NOT NULL DEFAULT 'rsa-sha256',
    public_key_pem TEXT NOT NULL,
    private_key_pem TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_actor_keys_one_active
    ON actor_keys (actor_id)
    WHERE active;

CREATE TABLE users (
    id UUID PRIMARY KEY REFERENCES actors(id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'worker',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_users_username_lower ON users (lower(username));

CREATE TABLE projects (
    id UUID PRIMARY KEY REFERENCES actors(id) ON DELETE CASCADE,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE project_members (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'manager', 'developer', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, project_id)
);

CREATE TABLE actor_follows (
    follower_actor_id UUID NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    followed_actor_id UUID NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    state TEXT NOT NULL DEFAULT 'accepted' CHECK (state IN ('pending', 'accepted', 'rejected')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_actor_id, followed_actor_id)
);

CREATE TABLE project_invites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ap_id TEXT NOT NULL UNIQUE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    inviter_actor_id UUID NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    invitee_actor_id UUID NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'manager', 'developer', 'viewer')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected', 'revoked')),
    invite_activity_id UUID,
    response_activity_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ap_id TEXT NOT NULL UNIQUE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    reporter_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'review', 'done')),
    priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high', 'urgent')),
    type TEXT NOT NULL DEFAULT 'task' CHECK (type IN ('epic', 'task', 'subtask')),
    parent_id UUID REFERENCES tickets(id) ON DELETE SET NULL,
    is_resolved BOOLEAN NOT NULL DEFAULT false,
    resolved_at TIMESTAMPTZ,
    resolved_by_actor_id UUID REFERENCES actors(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_ticket_not_own_parent CHECK (parent_id IS NULL OR parent_id <> id)
);

CREATE INDEX idx_tickets_project_id ON tickets(project_id);
CREATE INDEX idx_tickets_parent_id ON tickets(parent_id);
CREATE INDEX idx_tickets_status ON tickets(status);

CREATE TABLE ticket_assignees (
    ticket_id UUID NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    actor_id UUID NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (ticket_id, actor_id)
);

CREATE TABLE ticket_links (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id UUID NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    target_id UUID NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    link_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(source_id, target_id),
    CONSTRAINT chk_ticket_link_not_self CHECK (source_id <> target_id)
);

CREATE INDEX idx_ticket_links_source_id ON ticket_links(source_id);
CREATE INDEX idx_ticket_links_target_id ON ticket_links(target_id);

CREATE TABLE comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ap_id TEXT NOT NULL UNIQUE,
    ticket_id UUID NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_comments_ticket_id ON comments(ticket_id);

CREATE TABLE ap_objects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ap_id TEXT NOT NULL UNIQUE,
    object_type TEXT NOT NULL,
    actor_id UUID REFERENCES actors(id) ON DELETE SET NULL,
    local_ref_table TEXT,
    local_ref_id UUID,
    document JSONB NOT NULL,
    is_deleted BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ap_objects_type ON ap_objects(object_type);
CREATE INDEX idx_ap_objects_local_ref ON ap_objects(local_ref_table, local_ref_id);

CREATE TABLE ap_activities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ap_id TEXT NOT NULL UNIQUE,
    activity_type TEXT NOT NULL CHECK (activity_type IN ('Create', 'Update', 'Delete', 'Add', 'Remove', 'Invite', 'Accept', 'Reject', 'Follow', 'Undo')),
    actor_id UUID NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    object_ap_id TEXT,
    target_ap_id TEXT,
    document JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ap_activities_actor_id ON ap_activities(actor_id);
CREATE INDEX idx_ap_activities_type ON ap_activities(activity_type);

ALTER TABLE project_invites
    ADD CONSTRAINT fk_project_invites_invite_activity
    FOREIGN KEY (invite_activity_id) REFERENCES ap_activities(id) ON DELETE SET NULL;

ALTER TABLE project_invites
    ADD CONSTRAINT fk_project_invites_response_activity
    FOREIGN KEY (response_activity_id) REFERENCES ap_activities(id) ON DELETE SET NULL;

CREATE TABLE actor_inbox_items (
    actor_id UUID NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    activity_id UUID NOT NULL REFERENCES ap_activities(id) ON DELETE CASCADE,
    activity_ap_id TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_id, activity_id),
    UNIQUE(actor_id, activity_ap_id)
);

CREATE TABLE actor_outbox_items (
    actor_id UUID NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    activity_id UUID NOT NULL REFERENCES ap_activities(id) ON DELETE CASCADE,
    activity_ap_id TEXT NOT NULL,
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_id, activity_id),
    UNIQUE(actor_id, activity_ap_id)
);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_actors_updated_at BEFORE UPDATE ON actors
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_projects_updated_at BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_actor_follows_updated_at BEFORE UPDATE ON actor_follows
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_project_invites_updated_at BEFORE UPDATE ON project_invites
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_tickets_updated_at BEFORE UPDATE ON tickets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_comments_updated_at BEFORE UPDATE ON comments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_ap_objects_updated_at BEFORE UPDATE ON ap_objects
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE actors IS 'Local and remote ActivityPub actors. Local users are Person actors; projects are Group actors with ForgeFed semantics.';
COMMENT ON TABLE ap_objects IS 'Current JSON-LD snapshots for ActivityStreams and ForgeFed objects.';
COMMENT ON TABLE ap_activities IS 'Append-only local ActivityStreams activities used by inbox/outbox collections.';
