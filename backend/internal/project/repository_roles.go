package project

import (
	"context"
	"database/sql"
	"errors"

	"github.com/antonovs105/project-management-system-go/internal/apperror"
	"github.com/jmoiron/sqlx"
)

// ListMembers returns a bounded project collaborator page.
func (r *PgRepository) ListMembers(ctx context.Context, projectID string, options ProjectListOptions) ([]ProjectMember, error) {
	members := make([]ProjectMember, 0)
	if err := r.db.SelectContext(ctx, &members, projectCollaboratorSelectQuery()+`
		WHERE project_id = $1
		ORDER BY role_position ASC, created_at ASC, lower(name) ASC, user_id ASC
		LIMIT $2 OFFSET $3
	`, projectID, options.Limit, options.Offset); err != nil {
		return nil, err
	}
	return members, nil
}

// ListInvites returns project invites with actor and role metadata.
func (r *PgRepository) ListInvites(ctx context.Context, projectID string, options ProjectInviteListOptions) ([]ProjectInviteInspection, error) {
	invites := make([]ProjectInviteInspection, 0)
	if err := r.db.SelectContext(ctx, &invites, projectInviteInspectionSelectQuery()+`
		WHERE invite.project_id = $1
			AND ($2 = '' OR invite.status = $2)
		ORDER BY invite.created_at DESC, invite.id DESC
		LIMIT $3 OFFSET $4
	`, projectID, options.Status, options.Limit, options.Offset); err != nil {
		return nil, err
	}
	return invites, nil
}

// ListInvitesForActor returns project invites addressed to one actor.
func (r *PgRepository) ListInvitesForActor(ctx context.Context, actorID string, options ProjectInviteListOptions) ([]ProjectInviteInspection, error) {
	invites := make([]ProjectInviteInspection, 0)
	if err := r.db.SelectContext(ctx, &invites, projectInviteInspectionSelectQuery()+`
		WHERE invite.invitee_actor_id = $1
			AND ($2 = '' OR invite.status = $2)
		ORDER BY invite.created_at DESC, invite.id DESC
		LIMIT $3 OFFSET $4
	`, actorID, options.Status, options.Limit, options.Offset); err != nil {
		return nil, err
	}
	return invites, nil
}

// GetMemberRole returns a user's role key in a project.
func (r *PgRepository) GetMemberRole(ctx context.Context, userID, projectID string) (string, error) {
	var role string
	err := r.db.GetContext(ctx, &role, `
		SELECT project_role.key
		FROM project_members member
		JOIN project_roles project_role ON project_role.id = member.role_id
		WHERE member.user_id = $1 AND member.project_id = $2
	`, userID, projectID)
	return role, err
}

// HasPermission reports whether a user has a project permission through their project role.
func (r *PgRepository) HasPermission(ctx context.Context, projectID, userID, permission string) (bool, error) {
	var allowed bool
	err := r.db.GetContext(ctx, &allowed, `
		SELECT EXISTS(
			SELECT 1
			FROM project_members member
			JOIN project_role_permissions permission ON permission.role_id = member.role_id
			WHERE member.project_id = $1
				AND member.user_id = $2
				AND permission.permission = $3
		) OR EXISTS(
			SELECT 1
			FROM project_invites invite
			JOIN actors invitee ON invitee.id = invite.invitee_actor_id
			JOIN project_role_permissions permission ON permission.role_id = invite.role_id
			WHERE invite.project_id = $1
				AND invite.invitee_actor_id = $2
				AND invite.status = 'accepted'
				AND invitee.is_local = false
				AND permission.permission = $3
		)
	`, projectID, userID, permission)
	return allowed, err
}

// CountMembersWithPermission counts project members whose role grants permission.
func (r *PgRepository) CountMembersWithPermission(ctx context.Context, projectID, permission string) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count, `
		SELECT count(*)
		FROM (`+projectCollaboratorRolesSQL()+`) collaborator
		JOIN project_role_permissions permission ON permission.role_id = collaborator.role_id
		WHERE collaborator.project_id = $1
			AND permission.permission = $2
	`, projectID, permission)
	return count, err
}

// CountMembersWithPermissionExcludingRole counts members whose role grants permission, excluding one role.
func (r *PgRepository) CountMembersWithPermissionExcludingRole(ctx context.Context, projectID, permission, excludedRoleID string) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count, `
		SELECT count(*)
		FROM (`+projectCollaboratorRolesSQL()+`) collaborator
		JOIN project_role_permissions permission ON permission.role_id = collaborator.role_id
		WHERE collaborator.project_id = $1
			AND permission.permission = $2
			AND collaborator.role_id <> $3
	`, projectID, permission, excludedRoleID)
	return count, err
}

// RoleHasPermission reports whether a project role grants permission.
func (r *PgRepository) RoleHasPermission(ctx context.Context, roleID, permission string) (bool, error) {
	var allowed bool
	err := r.db.GetContext(ctx, &allowed, `
		SELECT EXISTS(
			SELECT 1
			FROM project_role_permissions
			WHERE role_id = $1 AND permission = $2
		)
	`, roleID, permission)
	return allowed, err
}

// ResolveRole finds a project role by id, key, or case-insensitive display name.
func (r *PgRepository) ResolveRole(ctx context.Context, projectID, roleRef string) (*ProjectRole, error) {
	var role ProjectRole
	if err := r.db.GetContext(ctx, &role, projectRoleSelectQuery()+`
		WHERE project_id = $1
			AND (id::text = $2 OR key = lower($2) OR lower(name) = lower($2))
	`, projectID, roleRef); err != nil {
		return nil, err
	}
	if err := loadRolePermissions(ctx, r.db, &role); err != nil {
		return nil, err
	}
	return &role, nil
}

// ListRoles returns the configurable roles for a project.
func (r *PgRepository) ListRoles(ctx context.Context, projectID string) ([]ProjectRole, error) {
	roles := make([]ProjectRole, 0)
	if err := r.db.SelectContext(ctx, &roles, projectRoleSelectQuery()+`
		WHERE project_id = $1
		ORDER BY position ASC, lower(name) ASC, id ASC
	`, projectID); err != nil {
		return nil, err
	}
	for i := range roles {
		if err := loadRolePermissions(ctx, r.db, &roles[i]); err != nil {
			return nil, err
		}
	}
	return roles, nil
}

// GetRoleByID loads a project role by UUID.
func (r *PgRepository) GetRoleByID(ctx context.Context, projectID, roleID string) (*ProjectRole, error) {
	var role ProjectRole
	if err := r.db.GetContext(ctx, &role, projectRoleSelectQuery()+`
		WHERE project_id = $1 AND id = $2
	`, projectID, roleID); err != nil {
		return nil, err
	}
	if err := loadRolePermissions(ctx, r.db, &role); err != nil {
		return nil, err
	}
	return &role, nil
}

// CreateRole stores a custom project role and its permissions.
func (r *PgRepository) CreateRole(ctx context.Context, role *ProjectRole) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := tx.QueryRowxContext(ctx, `
		INSERT INTO project_roles (project_id, key, name, description, is_system, position)
		VALUES ($1, $2, $3, $4, false, COALESCE((SELECT max(position) + 10 FROM project_roles WHERE project_id = $1), 10))
		RETURNING id::text, created_at, updated_at, position
	`, role.ProjectID, role.Key, role.Name, role.Description).Scan(&role.ID, &role.CreatedAt, &role.UpdatedAt, &role.Position); err != nil {
		return err
	}
	if err := replaceRolePermissions(ctx, tx, role.ID, role.Permissions); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateRole changes a role's display data and permission set.
func (r *PgRepository) UpdateRole(ctx context.Context, role *ProjectRole) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := lockProjectManagerBoundary(ctx, tx, role.ProjectID); err != nil {
		return err
	}
	if err := preventLastRoleManagerPermissionRemoval(ctx, tx, role); err != nil {
		return err
	}

	if err := tx.QueryRowxContext(ctx, `
		UPDATE project_roles
		SET name = $3,
			description = $4
		WHERE project_id = $1 AND id = $2
		RETURNING key, is_system, position, created_at, updated_at
	`, role.ProjectID, role.ID, role.Name, role.Description).Scan(
		&role.Key,
		&role.IsSystem,
		&role.Position,
		&role.CreatedAt,
		&role.UpdatedAt,
	); err != nil {
		return err
	}
	if err := replaceRolePermissions(ctx, tx, role.ID, role.Permissions); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteRole removes an unused custom project role.
func (r *PgRepository) DeleteRole(ctx context.Context, projectID, roleID string) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM project_roles
		WHERE project_id = $1
			AND id = $2
			AND is_system = false
	`, projectID, roleID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return apperror.New(apperror.ErrForbidden, "project role not found or protected")
	}
	return nil
}

// RoleAssignmentCount returns the number of memberships or pending invites using a role.
func (r *PgRepository) RoleAssignmentCount(ctx context.Context, projectID, roleID string) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count, `
		SELECT
			(SELECT count(*) FROM project_members WHERE project_id = $1 AND role_id = $2)
			+
			(SELECT count(*) FROM project_invites WHERE project_id = $1 AND role_id = $2 AND status IN ('pending', 'accepted'))
	`, projectID, roleID)
	return count, err
}

// ResolveInviteeActorID resolves an invite target by UUID, handle, username, or local email.
func (r *PgRepository) ResolveInviteeActorID(ctx context.Context, ref string) (string, error) {
	var actorID string
	err := r.db.GetContext(ctx, &actorID, `
		SELECT actor.id::text
		FROM actors actor
		LEFT JOIN users u ON u.id = actor.id
		WHERE actor.id::text = $1
			OR lower(actor.handle) = lower($1)
			OR lower('acct:' || actor.handle) = lower($1)
			OR lower(actor.preferred_username) = lower($1)
			OR lower(u.username) = lower($1)
			OR lower(u.email) = lower($1)
		ORDER BY actor.is_local DESC, actor.created_at DESC, actor.id ASC
		LIMIT 1
	`, ref)
	return actorID, err
}

// lockProjectManagerBoundary serializes changes that could remove the final project role manager.
func lockProjectManagerBoundary(ctx context.Context, tx *sqlx.Tx, projectID string) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "project-manager-boundary:"+projectID)
	return err
}

// preventLastRoleManagerPermissionRemoval protects project administration from lockout.
func preventLastRoleManagerPermissionRemoval(ctx context.Context, tx *sqlx.Tx, role *ProjectRole) error {
	var currentlyGrantsRoleManagement bool
	if err := tx.GetContext(ctx, &currentlyGrantsRoleManagement, `
		SELECT EXISTS(
			SELECT 1
			FROM project_role_permissions
			WHERE role_id = $1 AND permission = $2
		)
	`, role.ID, PermissionRolesManage); err != nil {
		return err
	}
	if !currentlyGrantsRoleManagement || hasPermission(role.Permissions, PermissionRolesManage) {
		return nil
	}

	var remainingManagers int
	if err := tx.GetContext(ctx, &remainingManagers, `
		SELECT count(*)
		FROM project_members member
		JOIN project_role_permissions permission ON permission.role_id = member.role_id
		WHERE member.project_id = $1
			AND member.role_id <> $2
			AND permission.permission = $3
	`, role.ProjectID, role.ID, PermissionRolesManage); err != nil {
		return err
	}
	if remainingManagers == 0 {
		return apperror.New(apperror.ErrForbidden, "cannot remove the last project role manager")
	}
	return nil
}

// IsProjectMember reports whether a user belongs to a project.
func (r *PgRepository) IsProjectMember(ctx context.Context, projectID, userID string) (bool, error) {
	var member bool
	err := r.db.GetContext(ctx, &member, `
		SELECT EXISTS(
			SELECT 1
			FROM project_members
			WHERE project_id = $1 AND user_id = $2
		) OR EXISTS(
			SELECT 1
			FROM project_invites invite
			JOIN actors invitee ON invitee.id = invite.invitee_actor_id
			WHERE invite.project_id = $1
				AND invite.invitee_actor_id = $2
				AND invite.status = 'accepted'
				AND invitee.is_local = false
		)
	`, projectID, userID)
	return member, err
}

// HasPendingInvite reports whether a user already has an open invite.
func (r *PgRepository) HasPendingInvite(ctx context.Context, projectID, userID string) (bool, error) {
	var pending bool
	err := r.db.GetContext(ctx, &pending, `
		SELECT EXISTS(
			SELECT 1
			FROM project_invites
			WHERE project_id = $1
				AND invitee_actor_id = $2
				AND status = 'pending'
		)
	`, projectID, userID)
	return pending, err
}

// projectRoleSelectQuery returns the shared role projection.
func projectRoleSelectQuery() string {
	return `
		SELECT
			id::text,
			project_id::text,
			key,
			name,
			description,
			is_system,
			position,
			created_at,
			updated_at
		FROM project_roles
	`
}

// projectMemberSelectQuery returns the shared member API projection.
func projectMemberSelectQuery() string {
	return `
		SELECT
			member.user_id::text,
			member.project_id::text,
			member.role_id::text,
			role.key AS role,
			role.name AS role_name,
			u.username,
			u.email,
			actor.handle,
			actor.name,
			false AS is_remote,
			member.created_at
		FROM project_members member
		JOIN users u ON u.id = member.user_id
		JOIN actors actor ON actor.id = member.user_id
		JOIN project_roles role ON role.id = member.role_id
	`
}

// projectCollaboratorSelectQuery returns local members plus accepted remote invite actors.
func projectCollaboratorSelectQuery() string {
	return `
		SELECT
			user_id,
			project_id,
			role_id,
			role,
			role_name,
			username,
			email,
			handle,
			name,
			is_remote,
			created_at
		FROM (
			SELECT
				member.user_id::text,
				member.project_id::text,
				member.role_id::text,
				role.key AS role,
				role.name AS role_name,
				u.username,
				u.email,
				actor.handle,
				actor.name,
				false AS is_remote,
				member.created_at,
				role.position AS role_position
			FROM project_members member
			JOIN users u ON u.id = member.user_id
			JOIN actors actor ON actor.id = member.user_id
			JOIN project_roles role ON role.id = member.role_id
			UNION ALL
			SELECT
				invite.invitee_actor_id::text AS user_id,
				invite.project_id::text,
				invite.role_id::text,
				role.key AS role,
				role.name AS role_name,
				COALESCE(invitee_user.username, invitee.preferred_username) AS username,
				COALESCE(invitee_user.email, '') AS email,
				invitee.handle,
				invitee.name,
				true AS is_remote,
				invite.updated_at AS created_at,
				role.position AS role_position
			FROM project_invites invite
			JOIN actors invitee ON invitee.id = invite.invitee_actor_id
			LEFT JOIN users invitee_user ON invitee_user.id = invite.invitee_actor_id
			JOIN project_roles role ON role.id = invite.role_id
			WHERE invite.status = 'accepted'
				AND invitee.is_local = false
				AND NOT EXISTS (
					SELECT 1
					FROM project_members member
					WHERE member.project_id = invite.project_id
						AND member.user_id = invite.invitee_actor_id
				)
		) collaborator
	`
}

// projectCollaboratorRolesSQL returns role assignments for local and accepted remote collaborators.
func projectCollaboratorRolesSQL() string {
	return `
		SELECT member.project_id, member.user_id::text AS actor_id, member.role_id
		FROM project_members member
		UNION ALL
		SELECT invite.project_id, invite.invitee_actor_id::text AS actor_id, invite.role_id
		FROM project_invites invite
		JOIN actors invitee ON invitee.id = invite.invitee_actor_id
		WHERE invite.status = 'accepted'
			AND invitee.is_local = false
			AND NOT EXISTS (
				SELECT 1
				FROM project_members member
				WHERE member.project_id = invite.project_id
					AND member.user_id = invite.invitee_actor_id
			)
	`
}

// projectInviteInspectionSelectQuery returns the shared invite inspection projection.
func projectInviteInspectionSelectQuery() string {
	return `
		SELECT
			invite.id::text,
			invite.ap_id,
			invite.project_id::text,
			project.name AS project_name,
			project_actor.handle AS project_handle,
			invite.inviter_actor_id::text,
			invite.invitee_actor_id::text,
			invite.role_id::text,
			role.key AS role,
			role.name AS role_name,
			invite.status,
			COALESCE(inviter_user.username, inviter.preferred_username) AS inviter_username,
			COALESCE(inviter_user.email, '') AS inviter_email,
			inviter.handle AS inviter_handle,
			inviter.name AS inviter_name,
			COALESCE(invitee_user.username, invitee.preferred_username) AS invitee_username,
			COALESCE(invitee_user.email, '') AS invitee_email,
			invitee.handle AS invitee_handle,
			invitee.name AS invitee_name,
			invite.created_at,
			invite.updated_at
		FROM project_invites invite
		JOIN projects project ON project.id = invite.project_id
		JOIN actors project_actor ON project_actor.id = invite.project_id
		JOIN actors inviter ON inviter.id = invite.inviter_actor_id
		JOIN actors invitee ON invitee.id = invite.invitee_actor_id
		LEFT JOIN users inviter_user ON inviter_user.id = invite.inviter_actor_id
		LEFT JOIN users invitee_user ON invitee_user.id = invite.invitee_actor_id
		JOIN project_roles role ON role.id = invite.role_id
	`
}

// loadProjectMember loads one member using the public member projection.
func loadProjectMember(ctx context.Context, q sqlx.QueryerContext, projectID, userID string) (*ProjectMember, error) {
	var member ProjectMember
	if err := sqlx.GetContext(ctx, q, &member, projectCollaboratorSelectQuery()+`
		WHERE project_id = $1 AND user_id = $2
		LIMIT 1
	`, projectID, userID); err != nil {
		return nil, err
	}
	return &member, nil
}

// collaboratorRole describes a locked local or accepted remote project role assignment.
type collaboratorRole struct {
	IsLocal   bool   `db:"is_local"`
	RoleID    string `db:"role_id"`
	Key       string `db:"key"`
	CanManage bool   `db:"can_manage"`
}

// collaboratorRoleForUpdate locks a local member or accepted remote invite row for mutation.
func collaboratorRoleForUpdate(ctx context.Context, tx *sqlx.Tx, projectID, actorID string) (*collaboratorRole, error) {
	var local collaboratorRole
	if err := tx.GetContext(ctx, &local, `
		SELECT
			true AS is_local,
			member.role_id::text,
			project_role.key,
			EXISTS(
				SELECT 1
				FROM project_role_permissions permission
				WHERE permission.role_id = member.role_id
					AND permission.permission = $3
			) AS can_manage
		FROM project_members member
		JOIN project_roles project_role ON project_role.id = member.role_id
		WHERE member.project_id = $1 AND member.user_id = $2
		FOR UPDATE OF member
	`, projectID, actorID, PermissionRolesManage); err == nil {
		return &local, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	var remote collaboratorRole
	if err := tx.GetContext(ctx, &remote, `
		SELECT
			false AS is_local,
			invite.role_id::text,
			project_role.key,
			EXISTS(
				SELECT 1
				FROM project_role_permissions permission
				WHERE permission.role_id = invite.role_id
					AND permission.permission = $3
			) AS can_manage
		FROM project_invites invite
		JOIN actors invitee ON invitee.id = invite.invitee_actor_id
		JOIN project_roles project_role ON project_role.id = invite.role_id
		WHERE invite.project_id = $1
			AND invite.invitee_actor_id = $2
			AND invite.status = 'accepted'
			AND invitee.is_local = false
			AND NOT EXISTS (
				SELECT 1
				FROM project_members member
				WHERE member.project_id = invite.project_id
					AND member.user_id = invite.invitee_actor_id
			)
		ORDER BY invite.updated_at DESC, invite.id DESC
		LIMIT 1
		FOR UPDATE OF invite
	`, projectID, actorID, PermissionRolesManage); err != nil {
		return nil, err
	}
	return &remote, nil
}

// countProjectManagersTx counts collaborators whose current role grants role management.
func countProjectManagersTx(ctx context.Context, tx *sqlx.Tx, projectID string) (int, error) {
	return countProjectManagersExceptActorTx(ctx, tx, projectID, "")
}

// countProjectManagersExceptActorTx counts role managers excluding one actor when provided.
func countProjectManagersExceptActorTx(ctx context.Context, tx *sqlx.Tx, projectID string, excludedActorID string) (int, error) {
	var managers int
	err := tx.GetContext(ctx, &managers, `
		SELECT count(*)
		FROM (`+projectCollaboratorRolesSQL()+`) collaborator
		JOIN project_role_permissions permission ON permission.role_id = collaborator.role_id
		WHERE collaborator.project_id = $1
			AND permission.permission = $2
			AND ($3 = '' OR collaborator.actor_id <> $3)
	`, projectID, PermissionRolesManage, excludedActorID)
	return managers, err
}

// loadRolePermissions attaches permissions to a role projection.
func loadRolePermissions(ctx context.Context, q sqlx.QueryerContext, role *ProjectRole) error {
	permissions := make([]string, 0)
	if err := sqlx.SelectContext(ctx, q, &permissions, `
		SELECT permission
		FROM project_role_permissions
		WHERE role_id = $1
		ORDER BY permission ASC
	`, role.ID); err != nil {
		return err
	}
	role.Permissions = permissions
	return nil
}

// replaceRolePermissions atomically replaces a role permission set.
func replaceRolePermissions(ctx context.Context, tx *sqlx.Tx, roleID string, permissions []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_role_permissions WHERE role_id = $1`, roleID); err != nil {
		return err
	}
	for _, permission := range permissions {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_role_permissions (role_id, permission)
			VALUES ($1, $2)
		`, roleID, permission); err != nil {
			return err
		}
	}
	return nil
}

// Update changes project metadata and records an Update activity.
