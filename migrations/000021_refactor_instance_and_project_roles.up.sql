ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_role_check;

ALTER TABLE users
    RENAME COLUMN role TO instance_role;

UPDATE users
SET instance_role = CASE instance_role
    WHEN 'admin' THEN 'owner'
    WHEN 'worker' THEN 'user'
    ELSE instance_role
END;

ALTER TABLE users
    ALTER COLUMN instance_role SET DEFAULT 'user',
    ADD CONSTRAINT users_instance_role_check CHECK (instance_role IN ('owner', 'admin', 'user'));

CREATE TABLE project_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    is_system BOOLEAN NOT NULL DEFAULT false,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT project_roles_key_not_blank CHECK (btrim(key) <> ''),
    CONSTRAINT project_roles_name_not_blank CHECK (btrim(name) <> '')
);

CREATE UNIQUE INDEX idx_project_roles_project_key
    ON project_roles(project_id, key);

CREATE UNIQUE INDEX idx_project_roles_project_name_lower
    ON project_roles(project_id, lower(name));

CREATE INDEX idx_project_roles_project_id
    ON project_roles(project_id);

CREATE TABLE project_role_permissions (
    role_id UUID NOT NULL REFERENCES project_roles(id) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    PRIMARY KEY (role_id, permission),
    CONSTRAINT project_role_permissions_permission_not_blank CHECK (btrim(permission) <> '')
);

CREATE TRIGGER trg_project_roles_updated_at BEFORE UPDATE ON project_roles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

WITH role_templates(key, name, description, is_system, position) AS (
    VALUES
        ('owner', 'Owner', 'Full project control, including roles and destructive actions.', true, 10),
        ('manager', 'Manager', 'Can manage project work and membership without deleting the project.', false, 20),
        ('developer', 'Developer', 'Can work with tickets and comments.', false, 30),
        ('viewer', 'Viewer', 'Can read project content only.', false, 40)
)
INSERT INTO project_roles (project_id, key, name, description, is_system, position)
SELECT p.id, t.key, t.name, t.description, t.is_system, t.position
FROM projects p
CROSS JOIN role_templates t
ON CONFLICT (project_id, key) DO NOTHING;

WITH permission_templates(role_key, permission) AS (
    VALUES
        ('owner', 'project.read'),
        ('owner', 'project.update'),
        ('owner', 'project.delete'),
        ('owner', 'members.invite'),
        ('owner', 'members.remove'),
        ('owner', 'roles.manage'),
        ('owner', 'tickets.create'),
        ('owner', 'tickets.update'),
        ('owner', 'tickets.delete'),
        ('owner', 'comments.create'),
        ('owner', 'comments.moderate'),
        ('owner', 'federation.delivery.retry'),
        ('manager', 'project.read'),
        ('manager', 'project.update'),
        ('manager', 'members.invite'),
        ('manager', 'members.remove'),
        ('manager', 'tickets.create'),
        ('manager', 'tickets.update'),
        ('manager', 'tickets.delete'),
        ('manager', 'comments.create'),
        ('manager', 'comments.moderate'),
        ('manager', 'federation.delivery.retry'),
        ('developer', 'project.read'),
        ('developer', 'tickets.create'),
        ('developer', 'tickets.update'),
        ('developer', 'comments.create'),
        ('viewer', 'project.read')
)
INSERT INTO project_role_permissions (role_id, permission)
SELECT pr.id, pt.permission
FROM project_roles pr
JOIN permission_templates pt ON pt.role_key = pr.key
ON CONFLICT (role_id, permission) DO NOTHING;

ALTER TABLE project_members
    ADD COLUMN role_id UUID;

UPDATE project_members member
SET role_id = role.id
FROM project_roles role
WHERE role.project_id = member.project_id
    AND role.key = member.role;

ALTER TABLE project_members
    ALTER COLUMN role_id SET NOT NULL,
    ADD CONSTRAINT fk_project_members_role
        FOREIGN KEY (role_id) REFERENCES project_roles(id) ON DELETE RESTRICT;

CREATE INDEX idx_project_members_role_id
    ON project_members(role_id);

ALTER TABLE project_members
    DROP CONSTRAINT IF EXISTS project_members_role_check,
    DROP COLUMN role;

ALTER TABLE project_invites
    ADD COLUMN role_id UUID;

UPDATE project_invites invite
SET role_id = role.id
FROM project_roles role
WHERE role.project_id = invite.project_id
    AND role.key = invite.role;

ALTER TABLE project_invites
    ALTER COLUMN role_id SET NOT NULL,
    ADD CONSTRAINT fk_project_invites_role
        FOREIGN KEY (role_id) REFERENCES project_roles(id) ON DELETE RESTRICT;

CREATE INDEX idx_project_invites_role_id
    ON project_invites(role_id);

ALTER TABLE project_invites
    DROP CONSTRAINT IF EXISTS project_invites_role_check,
    DROP COLUMN role;
