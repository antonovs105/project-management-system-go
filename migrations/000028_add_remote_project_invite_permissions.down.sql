DROP FUNCTION IF EXISTS remote_project_role_permissions(TEXT, TEXT[]);

ALTER TABLE remote_project_invites
    DROP COLUMN IF EXISTS role_permissions;
