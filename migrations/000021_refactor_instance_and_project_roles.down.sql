ALTER TABLE project_invites
    ADD COLUMN role TEXT;

UPDATE project_invites invite
SET role = role.key
FROM project_roles role
WHERE role.id = invite.role_id;

ALTER TABLE project_invites
    ALTER COLUMN role SET NOT NULL,
    ADD CONSTRAINT project_invites_role_check CHECK (role IN ('owner', 'manager', 'developer', 'viewer'));

ALTER TABLE project_invites
    DROP CONSTRAINT IF EXISTS fk_project_invites_role,
    DROP COLUMN role_id;

ALTER TABLE project_members
    ADD COLUMN role TEXT;

UPDATE project_members member
SET role = role.key
FROM project_roles role
WHERE role.id = member.role_id;

ALTER TABLE project_members
    ALTER COLUMN role SET NOT NULL,
    ADD CONSTRAINT project_members_role_check CHECK (role IN ('owner', 'manager', 'developer', 'viewer'));

ALTER TABLE project_members
    DROP CONSTRAINT IF EXISTS fk_project_members_role,
    DROP COLUMN role_id;

DROP TABLE IF EXISTS project_role_permissions;

DROP TRIGGER IF EXISTS trg_project_roles_updated_at ON project_roles;
DROP TABLE IF EXISTS project_roles;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_instance_role_check;

UPDATE users
SET instance_role = CASE instance_role
    WHEN 'owner' THEN 'admin'
    WHEN 'admin' THEN 'admin'
    ELSE 'worker'
END;

ALTER TABLE users
    ALTER COLUMN instance_role SET DEFAULT 'worker';

ALTER TABLE users
    RENAME COLUMN instance_role TO role;

ALTER TABLE users
    ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'worker'));
