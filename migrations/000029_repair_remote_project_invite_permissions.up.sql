ALTER TABLE remote_project_invites
    ADD COLUMN IF NOT EXISTS role_permissions TEXT[] NOT NULL DEFAULT '{}'::TEXT[];

CREATE OR REPLACE FUNCTION remote_project_role_permissions(role_key TEXT, stored_permissions TEXT[])
RETURNS TEXT[]
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT CASE
        WHEN cardinality(stored_permissions) > 0 THEN stored_permissions
        WHEN lower(btrim(role_key)) = 'owner' THEN ARRAY[
            'project.read',
            'project.update',
            'project.delete',
            'members.invite',
            'members.remove',
            'roles.manage',
            'tickets.create',
            'tickets.update',
            'tickets.delete',
            'comments.create',
            'comments.moderate',
            'federation.delivery.retry'
        ]::TEXT[]
        WHEN lower(btrim(role_key)) = 'manager' THEN ARRAY[
            'project.read',
            'project.update',
            'members.invite',
            'members.remove',
            'tickets.create',
            'tickets.update',
            'tickets.delete',
            'comments.create',
            'comments.moderate',
            'federation.delivery.retry'
        ]::TEXT[]
        WHEN lower(btrim(role_key)) = 'developer' THEN ARRAY[
            'project.read',
            'tickets.create',
            'tickets.update',
            'comments.create'
        ]::TEXT[]
        ELSE ARRAY['project.read']::TEXT[]
    END;
$$;
