package project

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/apperror"
	"github.com/antonovs105/project-management-system-go/internal/secrets"
	"github.com/jmoiron/sqlx"
)

// Repository defines persistence operations for projects, members, and invites.
type Repository interface {
	Create(ctx context.Context, project *Project) error
	GetByID(ctx context.Context, id string) (*Project, error)
	ListByOwnerID(ctx context.Context, ownerID string, options ProjectListOptions) ([]Project, error)
	ListMembers(ctx context.Context, projectID string, options ProjectListOptions) ([]ProjectMember, error)
	ListInvites(ctx context.Context, projectID string, options ProjectInviteListOptions) ([]ProjectInviteInspection, error)
	ListInvitesForActor(ctx context.Context, actorID string, options ProjectInviteListOptions) ([]ProjectInviteInspection, error)
	GetMemberRole(ctx context.Context, userID, projectID string) (string, error)
	HasPermission(ctx context.Context, projectID, userID, permission string) (bool, error)
	CountMembersWithPermission(ctx context.Context, projectID, permission string) (int, error)
	CountMembersWithPermissionExcludingRole(ctx context.Context, projectID, permission, excludedRoleID string) (int, error)
	RoleHasPermission(ctx context.Context, roleID, permission string) (bool, error)
	ResolveRole(ctx context.Context, projectID, roleRef string) (*ProjectRole, error)
	ListRoles(ctx context.Context, projectID string) ([]ProjectRole, error)
	GetRoleByID(ctx context.Context, projectID, roleID string) (*ProjectRole, error)
	CreateRole(ctx context.Context, role *ProjectRole) error
	UpdateRole(ctx context.Context, role *ProjectRole) error
	DeleteRole(ctx context.Context, projectID, roleID string) error
	RoleAssignmentCount(ctx context.Context, projectID, roleID string) (int, error)
	ResolveInviteeActorID(ctx context.Context, ref string) (string, error)
	IsProjectMember(ctx context.Context, projectID, userID string) (bool, error)
	HasPendingInvite(ctx context.Context, projectID, userID string) (bool, error)
	Update(ctx context.Context, project *Project, actorID string) (*UpdateResult, error)
	Delete(ctx context.Context, id string, actorID string) (*DeleteResult, error)
	RemoveMember(ctx context.Context, projectID, actorID, targetUserID string) (*MembershipResult, error)
	UpdateMemberRole(ctx context.Context, projectID, targetUserID, roleID string) (*ProjectMember, error)
	GetInviteByID(ctx context.Context, inviteID string) (*ProjectInvite, error)
	CreateInvite(ctx context.Context, invite *ProjectInvite) (*MembershipResult, error)
	AcceptInvite(ctx context.Context, inviteID, userID string) (*MembershipResult, error)
	RejectInvite(ctx context.Context, inviteID, userID string) (*MembershipResult, error)
	RevokeInvite(ctx context.Context, inviteID, actorID string) (*MembershipResult, error)
}

// PgRepository implements Repository using PostgreSQL.
type PgRepository struct {
	db              *sqlx.DB
	cfg             activitypub.Config
	privateKeyCodec secrets.PrivateKeyCodec
}

// NewRepository creates a PostgreSQL-backed project repository.
func NewRepository(db *sqlx.DB, cfg activitypub.Config, codecs ...secrets.PrivateKeyCodec) Repository {
	var privateKeyCodec secrets.PrivateKeyCodec = secrets.NoopPrivateKeyCodec{}
	if len(codecs) > 0 && codecs[0] != nil {
		privateKeyCodec = codecs[0]
	}
	return &PgRepository{db: db, cfg: cfg, privateKeyCodec: privateKeyCodec}
}

// Create stores a project actor, owner membership, and Create activity.
func (r *PgRepository) Create(ctx context.Context, project *Project) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setProjectMutationActor(ctx, tx, project.OwnerID); err != nil {
		return err
	}

	actorQuery := `
		INSERT INTO actors (
			id, ap_id, type, preferred_username, handle, name, summary,
			inbox_url, outbox_url, followers_url, following_url
		)
		VALUES (
			:id, :ap_id, 'Group', :preferred_username, :handle, :name, :summary,
			:inbox_url, :outbox_url, :followers_url, NULL
		)
	`
	actorParams := map[string]any{
		"id":                 project.ID,
		"ap_id":              project.APID,
		"preferred_username": "project-" + project.ID,
		"handle":             project.Handle,
		"name":               project.Name,
		"summary":            project.Description,
		"inbox_url":          activitypub.Inbox(project.APID),
		"outbox_url":         activitypub.Outbox(project.APID),
		"followers_url":      activitypub.Followers(project.APID),
	}
	if _, err := tx.NamedExecContext(ctx, actorQuery, actorParams); err != nil {
		return err
	}

	projectQuery := `
		INSERT INTO projects (id, name, description, owner_id)
		VALUES (:id, :name, :description, :owner_id)
	`
	if _, err := tx.NamedExecContext(ctx, projectQuery, project); err != nil {
		return err
	}
	if err := tx.QueryRowxContext(ctx, `
		SELECT created_at, updated_at FROM projects WHERE id = $1
	`, project.ID).Scan(&project.CreatedAt, &project.UpdatedAt); err != nil {
		return err
	}

	storedPrivateKey, err := r.privateKeyCodec.EncryptPrivateKey(project.PrivateKeyPEM)
	if err != nil {
		return err
	}
	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO actor_keys (actor_id, key_id, public_key_pem, private_key_pem)
		VALUES (:actor_id, :key_id, :public_key_pem, :private_key_pem)
	`, map[string]any{
		"actor_id":        project.ID,
		"key_id":          activitypub.KeyID(project.APID),
		"public_key_pem":  project.PublicKeyPEM,
		"private_key_pem": storedPrivateKey,
	}); err != nil {
		return err
	}

	defaultRoleIDs, err := insertDefaultProjectRoles(ctx, tx, project.ID)
	if err != nil {
		return err
	}
	ownerRoleID, ok := defaultRoleIDs[RoleOwner]
	if !ok {
		return errors.New("default owner role was not created")
	}
	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO project_members (user_id, project_id, role_id)
		VALUES (:user_id, :project_id, :role_id)
	`, map[string]any{"user_id": project.OwnerID, "project_id": project.ID, "role_id": ownerRoleID}); err != nil {
		return err
	}

	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
		VALUES (:follower_actor_id, :followed_actor_id, 'accepted')
	`, map[string]any{"follower_actor_id": project.OwnerID, "followed_actor_id": project.ID}); err != nil {
		return err
	}

	projectDoc := activitypub.ProjectActorDocument(project.APID, project.Name, project.Description, project.PublicKeyPEM)
	projectRaw, err := json.Marshal(projectDoc)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_objects (ap_id, object_type, actor_id, local_ref_table, local_ref_id, document)
		VALUES ($1, 'Group', $2, 'projects', $3, $4)
	`, project.APID, project.ID, project.ID, projectRaw); err != nil {
		return err
	}

	activityID, err := activitypub.NewID()
	if err != nil {
		return err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	ownerAPID, err := lookupActorAPID(ctx, tx, project.OwnerID)
	if err != nil {
		return err
	}
	activityDoc := activitypub.ActivityDocument("Create", activityAPID, ownerAPID, projectDoc, project.APID, time.Now().UTC())
	activityRaw, err := json.Marshal(activityDoc)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Create', $3, $4, $5, $6)
	`, activityID, activityAPID, project.OwnerID, project.APID, project.APID, activityRaw); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, project.OwnerID, activityID, activityAPID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, project.ID, activityID, activityAPID); err != nil {
		return err
	}

	return tx.Commit()
}

// insertDefaultProjectRoles creates the initial project-local roles and permissions.
func insertDefaultProjectRoles(ctx context.Context, tx *sqlx.Tx, projectID string) (map[string]string, error) {
	roleIDs := make(map[string]string, len(DefaultProjectRoles))
	for _, role := range DefaultProjectRoles {
		var roleID string
		if err := tx.QueryRowxContext(ctx, `
			INSERT INTO project_roles (project_id, key, name, description, is_system, position)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id::text
		`, projectID, role.Key, role.Name, role.Description, role.IsSystem, role.Position).Scan(&roleID); err != nil {
			return nil, err
		}
		roleIDs[role.Key] = roleID
		for _, permission := range role.Permissions {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO project_role_permissions (role_id, permission)
				VALUES ($1, $2)
			`, roleID, permission); err != nil {
				return nil, err
			}
		}
	}
	return roleIDs, nil
}

// GetByID loads a project by UUID.
func (r *PgRepository) GetByID(ctx context.Context, id string) (*Project, error) {
	var p Project
	query := `
		SELECT
			p.id::text,
			a.ap_id,
			p.name,
			p.description,
			p.owner_id::text,
			a.handle,
			p.version,
			p.archived_at,
			p.created_at,
			p.updated_at
		FROM projects p
		JOIN actors a ON a.id = p.id
		WHERE p.id = $1
	`
	err := r.db.GetContext(ctx, &p, query, id)
	return &p, err
}

// ListByOwnerID returns projects where the user is a member.
func (r *PgRepository) ListByOwnerID(ctx context.Context, ownerID string, options ProjectListOptions) ([]Project, error) {
	projects := make([]Project, 0)
	query := `
		SELECT
			p.id::text,
			a.ap_id,
			p.name,
			p.description,
			p.owner_id::text,
			a.handle,
			p.version,
			p.archived_at,
			p.created_at,
			p.updated_at
		FROM projects p
		JOIN actors a ON a.id = p.id
		JOIN project_members pm ON pm.project_id = p.id
		WHERE pm.user_id = $1 AND p.archived_at IS NULL
		ORDER BY p.created_at DESC
		LIMIT $2 OFFSET $3
	`
	if err := r.db.SelectContext(ctx, &projects, query, ownerID, options.Limit, options.Offset); err != nil {
		return nil, err
	}
	return projects, nil
}

// ListMembers returns local project members with role and profile metadata.
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
func (r *PgRepository) Update(ctx context.Context, project *Project, actorID string) (*UpdateResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := setProjectMutationActor(ctx, tx, actorID); err != nil {
		return nil, err
	}

	recipientInboxes, err := remoteProjectFollowerInboxes(ctx, tx, project.ID)
	if err != nil {
		return nil, err
	}

	result, err := tx.NamedExecContext(ctx, `
		UPDATE projects
		SET name = :name, description = :description
		WHERE id = :id AND version = :version
	`, project)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, apperror.New(apperror.ErrPrecondition, "project was changed by another request")
	}

	if _, err := tx.NamedExecContext(ctx, `
		UPDATE actors
		SET name = :name, summary = :description
		WHERE id = :id
	`, project); err != nil {
		return nil, err
	}

	publicKey, _ := lookupActivePublicKey(ctx, tx, project.ID)
	doc := activitypub.ProjectActorDocument(project.APID, project.Name, project.Description, publicKey)
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE ap_objects
		SET document = $1, object_type = 'Group'
		WHERE ap_id = $2
	`, rawDoc, project.APID); err != nil {
		return nil, err
	}

	activityID, err := r.writeProjectUpdateActivity(ctx, tx, project.ID, actorID, project.APID, doc)
	if err != nil {
		return nil, err
	}
	deliveries, err := apdelivery.CreateRowsForInboxes(ctx, tx, activityID, "", recipientInboxes, apdelivery.DefaultMaxRetry)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &UpdateResult{
		ActivityID:       activityID,
		ProjectID:        project.ID,
		RecipientInboxes: recipientInboxes,
		Deliveries:       deliveries,
	}, nil
}

// setProjectMutationActor makes trigger-based user history attributable within one transaction.
func setProjectMutationActor(ctx context.Context, tx *sqlx.Tx, actorID string) error {
	_, err := tx.ExecContext(ctx, `SELECT set_config('progo.actor_id', $1, true)`, actorID)
	return err
}

// Delete removes a project and tombstones its ActivityPub objects.
func (r *PgRepository) Delete(ctx context.Context, id string, actorID string) (*DeleteResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var stored struct {
		ID   string `db:"id"`
		APID string `db:"ap_id"`
	}
	if err := tx.GetContext(ctx, &stored, `
		SELECT project.id::text, actor.ap_id
		FROM projects project
		JOIN actors actor ON actor.id = project.id
		WHERE project.id = $1
		FOR UPDATE OF project
	`, id); err != nil {
		return nil, apperror.New(apperror.ErrNotFound, "project to delete not found")
	}

	recipientInboxes, err := remoteProjectFollowerInboxes(ctx, tx, stored.ID)
	if err != nil {
		return nil, err
	}
	if err := tombstoneProjectTree(ctx, tx, stored.ID, stored.APID); err != nil {
		return nil, err
	}
	activityID, err := r.writeProjectDeleteActivity(ctx, tx, stored.ID, actorID, stored.APID)
	if err != nil {
		return nil, err
	}
	deliveries, err := apdelivery.CreateRowsForInboxes(ctx, tx, activityID, "", recipientInboxes, apdelivery.DefaultMaxRetry)
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM actor_follows
		WHERE followed_actor_id = $1
	`, stored.ID); err != nil {
		return nil, err
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, stored.ID)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, apperror.New(apperror.ErrNotFound, "project to delete not found")
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &DeleteResult{
		ActivityID:       activityID,
		ProjectID:        stored.ID,
		RecipientInboxes: recipientInboxes,
		Deliveries:       deliveries,
	}, nil
}

// remoteProjectFollowerInboxes returns remote inboxes for accepted project followers.
func remoteProjectFollowerInboxes(ctx context.Context, q sqlx.QueryerContext, projectID string) ([]string, error) {
	inboxes := make([]string, 0)
	err := sqlx.SelectContext(ctx, q, &inboxes, `
		SELECT DISTINCT follower.inbox_url
		FROM actor_follows follow
		JOIN actors follower ON follower.id = follow.follower_actor_id
		WHERE follow.followed_actor_id = $1
			AND follow.state = 'accepted'
			AND follower.is_local = false
			AND follower.inbox_url <> ''
		ORDER BY follower.inbox_url ASC
	`, projectID)
	return inboxes, err
}

// remoteActorInboxes returns inbox URLs for a known remote actor.
func remoteActorInboxes(ctx context.Context, q sqlx.QueryerContext, actorID string) ([]string, error) {
	inboxes := make([]string, 0)
	err := sqlx.SelectContext(ctx, q, &inboxes, `
		SELECT inbox_url
		FROM actors
		WHERE id = $1
			AND is_local = false
			AND inbox_url <> ''
		ORDER BY inbox_url ASC
	`, actorID)
	return inboxes, err
}

// mergeInboxes merges inbox URL groups while preserving first-seen order.
func mergeInboxes(groups ...[]string) []string {
	seen := make(map[string]struct{})
	merged := make([]string, 0)
	for _, group := range groups {
		for _, inbox := range group {
			if inbox == "" {
				continue
			}
			if _, ok := seen[inbox]; ok {
				continue
			}
			seen[inbox] = struct{}{}
			merged = append(merged, inbox)
		}
	}
	return merged
}

// writeProjectUpdateActivity stores an ActivityStreams Update for project metadata.
func (r *PgRepository) writeProjectUpdateActivity(ctx context.Context, tx *sqlx.Tx, projectID, actorID, projectAPID string, object any) (string, error) {
	actorAPID, err := lookupActorAPID(ctx, tx, actorID)
	if err != nil {
		return "", err
	}
	activityID, err := activitypub.NewID()
	if err != nil {
		return "", err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	doc := activitypub.ActivityDocument("Update", activityAPID, actorAPID, object, projectAPID, time.Now().UTC())
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Update', $3, $4, $5, $6)
	`, activityID, activityAPID, actorID, projectAPID, projectAPID, rawDoc); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, actorID, activityID, activityAPID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, projectID, activityID, activityAPID); err != nil {
		return "", err
	}
	return activityID, nil
}

// tombstoneProjectTree tombstones a project actor and all contained ticket objects.
func tombstoneProjectTree(ctx context.Context, tx *sqlx.Tx, projectID, projectAPID string) error {
	commentAPIDs := make([]string, 0)
	if err := tx.SelectContext(ctx, &commentAPIDs, `
		SELECT comment.ap_id
		FROM comments comment
		JOIN tickets ticket ON ticket.id = comment.ticket_id
		WHERE ticket.project_id = $1
		ORDER BY comment.created_at ASC, comment.id ASC
	`, projectID); err != nil {
		return err
	}
	for _, apID := range commentAPIDs {
		if err := tombstoneObject(ctx, tx, apID, "Note"); err != nil {
			return err
		}
	}

	ticketAPIDs := make([]string, 0)
	if err := tx.SelectContext(ctx, &ticketAPIDs, `
		SELECT ap_id
		FROM tickets
		WHERE project_id = $1
		ORDER BY created_at ASC, id ASC
	`, projectID); err != nil {
		return err
	}
	for _, apID := range ticketAPIDs {
		if err := tombstoneObject(ctx, tx, apID, "forge:Ticket"); err != nil {
			return err
		}
	}

	return tombstoneObject(ctx, tx, projectAPID, "Group")
}

// writeProjectDeleteActivity stores an ActivityStreams Delete for a project actor.
func (r *PgRepository) writeProjectDeleteActivity(ctx context.Context, tx *sqlx.Tx, projectID, actorID, projectAPID string) (string, error) {
	actorAPID, err := lookupActorAPID(ctx, tx, actorID)
	if err != nil {
		return "", err
	}
	activityID, err := activitypub.NewID()
	if err != nil {
		return "", err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	doc := activitypub.ActivityDocument("Delete", activityAPID, actorAPID, projectAPID, projectAPID, time.Now().UTC())
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Delete', $3, $4, $5, $6)
	`, activityID, activityAPID, actorID, projectAPID, projectAPID, rawDoc); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, actorID, activityID, activityAPID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, projectID, activityID, activityAPID); err != nil {
		return "", err
	}
	return activityID, nil
}

// UpdateMemberRole changes a local project member's role without breaking manager lockout protection.
func (r *PgRepository) UpdateMemberRole(ctx context.Context, projectID, targetUserID, roleID string) (*ProjectMember, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := lockProjectManagerBoundary(ctx, tx, projectID); err != nil {
		return nil, err
	}

	var roleExists bool
	if err := tx.GetContext(ctx, &roleExists, `
		SELECT EXISTS(
			SELECT 1
			FROM project_roles
			WHERE project_id = $1 AND id = $2
		)
	`, projectID, roleID); err != nil {
		return nil, err
	}
	if !roleExists {
		return nil, apperror.New(apperror.ErrNotFound, "project role not found")
	}

	targetRole, err := collaboratorRoleForUpdate(ctx, tx, projectID, targetUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperror.New(apperror.ErrNotFound, "project member not found")
		}
		return nil, err
	}

	currentGrantsRoleManagement := targetRole.CanManage
	var nextGrantsRoleManagement bool
	if err := tx.GetContext(ctx, &nextGrantsRoleManagement, `
		SELECT EXISTS(
			SELECT 1
			FROM project_role_permissions
			WHERE role_id = $1 AND permission = $2
		)
	`, roleID, PermissionRolesManage); err != nil {
		return nil, err
	}
	if currentGrantsRoleManagement && !nextGrantsRoleManagement {
		remainingManagers, err := countProjectManagersExceptActorTx(ctx, tx, projectID, targetUserID)
		if err != nil {
			return nil, err
		}
		if remainingManagers == 0 {
			return nil, apperror.New(apperror.ErrForbidden, "cannot remove the last project role manager")
		}
	}

	if targetRole.IsLocal {
		if _, err := tx.ExecContext(ctx, `
			UPDATE project_members
			SET role_id = $3
			WHERE project_id = $1 AND user_id = $2
		`, projectID, targetUserID, roleID); err != nil {
			return nil, err
		}
	} else if _, err := tx.ExecContext(ctx, `
		UPDATE project_invites
		SET role_id = $3
		WHERE project_id = $1
			AND invitee_actor_id = $2
			AND status = 'accepted'
	`, projectID, targetUserID, roleID); err != nil {
		return nil, err
	}

	member, err := loadProjectMember(ctx, tx, projectID, targetUserID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return member, nil
}

// RemoveMember removes a user from a project and records membership activity.
func (r *PgRepository) RemoveMember(ctx context.Context, projectID, actorID, targetUserID string) (*MembershipResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := lockProjectManagerBoundary(ctx, tx, projectID); err != nil {
		return nil, err
	}

	targetRole, err := collaboratorRoleForUpdate(ctx, tx, projectID, targetUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperror.New(apperror.ErrConflict, "target user is not a project member")
		}
		return nil, err
	}

	followerInboxes, err := remoteProjectFollowerInboxes(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}
	targetInboxes, err := remoteActorInboxes(ctx, tx, targetUserID)
	if err != nil {
		return nil, err
	}
	recipientInboxes := mergeInboxes(followerInboxes, targetInboxes)

	if targetRole.CanManage {
		managers, err := countProjectManagersTx(ctx, tx, projectID)
		if err != nil {
			return nil, err
		}
		if managers <= 1 {
			return nil, apperror.New(apperror.ErrForbidden, "cannot remove the last project role manager")
		}

		var storedOwnerID string
		if err := tx.GetContext(ctx, &storedOwnerID, `SELECT owner_id::text FROM projects WHERE id = $1`, projectID); err != nil {
			return nil, err
		}
		if storedOwnerID == targetUserID {
			var nextOwnerID string
			if err := tx.GetContext(ctx, &nextOwnerID, `
				SELECT member.user_id::text
				FROM project_members member
				JOIN project_role_permissions permission ON permission.role_id = member.role_id
				WHERE member.project_id = $1
					AND member.user_id <> $2
					AND permission.permission = $3
				ORDER BY member.created_at ASC, member.user_id ASC
				LIMIT 1
			`, projectID, targetUserID, PermissionRolesManage); err != nil {
				return nil, err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE projects
				SET owner_id = $2
				WHERE id = $1
			`, projectID, nextOwnerID); err != nil {
				return nil, err
			}
		}
	}

	affectedTickets, err := removeTicketAssigneeForProjectMember(ctx, tx, projectID, targetUserID)
	if err != nil {
		return nil, err
	}
	for _, ticket := range affectedTickets {
		if err := updateTicketAssignedToDocument(ctx, tx, ticket.ID, ticket.APID); err != nil {
			return nil, err
		}
	}

	if targetRole.IsLocal {
		result, err := tx.ExecContext(ctx, `
			DELETE FROM project_members
			WHERE project_id = $1 AND user_id = $2
		`, projectID, targetUserID)
		if err != nil {
			return nil, err
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if rowsAffected == 0 {
			return nil, apperror.New(apperror.ErrConflict, "target user is not a project member")
		}
	} else {
		result, err := tx.ExecContext(ctx, `
			UPDATE project_invites
			SET status = 'revoked'
			WHERE project_id = $1
				AND invitee_actor_id = $2
				AND status = 'accepted'
		`, projectID, targetUserID)
		if err != nil {
			return nil, err
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if rowsAffected == 0 {
			return nil, apperror.New(apperror.ErrConflict, "target user is not a project member")
		}
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM actor_follows
		WHERE follower_actor_id = $1 AND followed_actor_id = $2
	`, targetUserID, projectID); err != nil {
		return nil, err
	}

	activityID, err := r.writeMemberRemovalActivity(ctx, tx, projectID, actorID, targetUserID)
	if err != nil {
		return nil, err
	}
	deliveries, err := apdelivery.CreateRowsForInboxes(ctx, tx, activityID, "", recipientInboxes, apdelivery.DefaultMaxRetry)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &MembershipResult{
		ActivityID:       activityID,
		ProjectID:        projectID,
		RecipientInboxes: recipientInboxes,
		Deliveries:       deliveries,
	}, nil
}

// GetInviteByID loads a project invite by UUID.
func (r *PgRepository) GetInviteByID(ctx context.Context, inviteID string) (*ProjectInvite, error) {
	var invite ProjectInvite
	if err := r.db.GetContext(ctx, &invite, `
		SELECT
			invite.id::text,
			invite.ap_id,
			invite.project_id::text,
			invite.inviter_actor_id::text,
			invite.invitee_actor_id::text,
			invite.role_id::text,
			project_role.key AS role,
			invite.status,
			invite.created_at,
			invite.updated_at
		FROM project_invites invite
		JOIN project_roles project_role ON project_role.id = invite.role_id
		WHERE invite.id = $1
	`, inviteID); err != nil {
		return nil, err
	}
	return &invite, nil
}

// CreateInvite stores an invite and its ActivityPub Invite activity.
func (r *PgRepository) CreateInvite(ctx context.Context, invite *ProjectInvite) (*MembershipResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	projectAPID, err := lookupActorAPID(ctx, tx, invite.ProjectID)
	if err != nil {
		return nil, err
	}
	var projectName string
	if err := tx.GetContext(ctx, &projectName, `
		SELECT name
		FROM projects
		WHERE id = $1
	`, invite.ProjectID); err != nil {
		return nil, err
	}
	rolePermissions := make([]string, 0)
	if err := tx.SelectContext(ctx, &rolePermissions, `
		SELECT permission
		FROM project_role_permissions
		WHERE role_id = $1
		ORDER BY permission ASC
	`, invite.RoleID); err != nil {
		return nil, err
	}
	inviterAPID, err := lookupActorAPID(ctx, tx, invite.InviterActorID)
	if err != nil {
		return nil, err
	}
	inviteeAPID, err := lookupActorAPID(ctx, tx, invite.InviteeActorID)
	if err != nil {
		return nil, err
	}
	recipientInboxes, err := remoteActorInboxes(ctx, tx, invite.InviteeActorID)
	if err != nil {
		return nil, err
	}

	member, err := isProjectMemberTx(ctx, tx, invite.ProjectID, invite.InviteeActorID)
	if err != nil {
		return nil, err
	}
	if member {
		return nil, apperror.New(apperror.ErrConflict, "user is already a project member")
	}
	pending, err := hasPendingInviteTx(ctx, tx, invite.ProjectID, invite.InviteeActorID)
	if err != nil {
		return nil, err
	}
	if pending {
		return nil, apperror.New(apperror.ErrConflict, "pending invite already exists")
	}

	activityID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	invite.APID = activityAPID

	object := map[string]any{
		"type":        "Group",
		"id":          projectAPID,
		"name":        projectName,
		"target":      inviteeAPID,
		"role":        invite.Role,
		"permissions": rolePermissions,
	}
	doc := activitypub.ActivityDocument("Invite", activityAPID, inviterAPID, object, inviteeAPID, time.Now().UTC())
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Invite', $3, $4, $5, $6)
	`, activityID, activityAPID, invite.InviterActorID, projectAPID, inviteeAPID, rawDoc); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, invite.InviterActorID, activityID, activityAPID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, invite.InviteeActorID, activityID, activityAPID); err != nil {
		return nil, err
	}

	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO project_invites (
			id, ap_id, project_id, inviter_actor_id, invitee_actor_id, role_id, status, invite_activity_id
		)
		VALUES (
			:id, :ap_id, :project_id, :inviter_actor_id, :invitee_actor_id, :role_id, 'pending', :invite_activity_id
		)
	`, map[string]any{
		"id":                 invite.ID,
		"ap_id":              invite.APID,
		"project_id":         invite.ProjectID,
		"inviter_actor_id":   invite.InviterActorID,
		"invitee_actor_id":   invite.InviteeActorID,
		"role_id":            invite.RoleID,
		"invite_activity_id": activityID,
	}); err != nil {
		return nil, err
	}
	deliveries, err := apdelivery.CreateRowsForInboxes(ctx, tx, activityID, "", recipientInboxes, apdelivery.DefaultMaxRetry)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &MembershipResult{
		ActivityID:       activityID,
		ProjectID:        invite.ProjectID,
		RecipientInboxes: recipientInboxes,
		Deliveries:       deliveries,
	}, nil
}

// AcceptInvite accepts a pending invite and creates membership records.
func (r *PgRepository) AcceptInvite(ctx context.Context, inviteID, userID string) (*MembershipResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var invite ProjectInvite
	if err := tx.GetContext(ctx, &invite, `
		SELECT
			invite.id::text,
			invite.ap_id,
			invite.project_id::text,
			invite.inviter_actor_id::text,
			invite.invitee_actor_id::text,
			invite.role_id::text,
			project_role.key AS role,
			invite.status,
			invite.created_at,
			invite.updated_at
		FROM project_invites invite
		JOIN project_roles project_role ON project_role.id = invite.role_id
		WHERE invite.id = $1
	`, inviteID); err != nil {
		return nil, err
	}
	if invite.InviteeActorID != userID {
		return nil, apperror.New(apperror.ErrForbidden, "invite does not belong to current user")
	}
	if invite.Status != "pending" {
		return nil, apperror.New(apperror.ErrConflict, "invite is not pending")
	}
	member, err := isProjectMemberTx(ctx, tx, invite.ProjectID, userID)
	if err != nil {
		return nil, err
	}
	if member {
		return nil, apperror.New(apperror.ErrConflict, "user is already a project member")
	}

	followerInboxes, err := remoteProjectFollowerInboxes(ctx, tx, invite.ProjectID)
	if err != nil {
		return nil, err
	}
	inviterInboxes, err := remoteActorInboxes(ctx, tx, invite.InviterActorID)
	if err != nil {
		return nil, err
	}
	recipientInboxes := mergeInboxes(followerInboxes, inviterInboxes)

	actorAPID, err := lookupActorAPID(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	projectAPID, err := lookupActorAPID(ctx, tx, invite.ProjectID)
	if err != nil {
		return nil, err
	}
	activityID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	doc := activitypub.ActivityDocument("Accept", activityAPID, actorAPID, invite.APID, projectAPID, time.Now().UTC())
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Accept', $3, $4, $5, $6)
	`, activityID, activityAPID, userID, invite.APID, projectAPID, rawDoc); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, userID, activityID, activityAPID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, invite.ProjectID, activityID, activityAPID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_members (user_id, project_id, role_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, project_id) DO UPDATE SET role_id = EXCLUDED.role_id
	`, userID, invite.ProjectID, invite.RoleID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
		VALUES ($1, $2, 'accepted')
		ON CONFLICT (follower_actor_id, followed_actor_id)
		DO UPDATE SET state = 'accepted'
	`, userID, invite.ProjectID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE project_invites
		SET status = 'accepted', response_activity_id = $1
		WHERE id = $2
	`, activityID, invite.ID); err != nil {
		return nil, err
	}
	deliveries, err := apdelivery.CreateRowsForInboxes(ctx, tx, activityID, "", recipientInboxes, apdelivery.DefaultMaxRetry)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &MembershipResult{
		ActivityID:       activityID,
		ProjectID:        invite.ProjectID,
		RecipientInboxes: recipientInboxes,
		Deliveries:       deliveries,
	}, nil
}

// RejectInvite rejects a pending invite and records a Reject activity.
func (r *PgRepository) RejectInvite(ctx context.Context, inviteID, userID string) (*MembershipResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	invite, err := getInviteForUpdate(ctx, tx, inviteID)
	if err != nil {
		return nil, err
	}
	if invite.InviteeActorID != userID {
		return nil, apperror.New(apperror.ErrForbidden, "invite does not belong to current user")
	}
	if invite.Status != "pending" {
		return nil, apperror.New(apperror.ErrConflict, "invite is not pending")
	}

	recipientInboxes, err := remoteActorInboxes(ctx, tx, invite.InviterActorID)
	if err != nil {
		return nil, err
	}

	actorAPID, err := lookupActorAPID(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	projectAPID, err := lookupActorAPID(ctx, tx, invite.ProjectID)
	if err != nil {
		return nil, err
	}
	activityID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	doc := activitypub.ActivityDocument("Reject", activityAPID, actorAPID, invite.APID, projectAPID, time.Now().UTC())
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Reject', $3, $4, $5, $6)
	`, activityID, activityAPID, userID, invite.APID, projectAPID, rawDoc); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, userID, activityID, activityAPID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, invite.ProjectID, activityID, activityAPID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE project_invites
		SET status = 'rejected', response_activity_id = $1
		WHERE id = $2
	`, activityID, invite.ID); err != nil {
		return nil, err
	}
	deliveries, err := apdelivery.CreateRowsForInboxes(ctx, tx, activityID, "", recipientInboxes, apdelivery.DefaultMaxRetry)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &MembershipResult{
		ActivityID:       activityID,
		ProjectID:        invite.ProjectID,
		RecipientInboxes: recipientInboxes,
		Deliveries:       deliveries,
	}, nil
}

// RevokeInvite revokes a pending invite and records an Undo activity.
func (r *PgRepository) RevokeInvite(ctx context.Context, inviteID, actorID string) (*MembershipResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	invite, err := getInviteForUpdate(ctx, tx, inviteID)
	if err != nil {
		return nil, err
	}
	if invite.Status != "pending" {
		return nil, apperror.New(apperror.ErrConflict, "invite is not pending")
	}

	recipientInboxes, err := remoteActorInboxes(ctx, tx, invite.InviteeActorID)
	if err != nil {
		return nil, err
	}

	actorAPID, err := lookupActorAPID(ctx, tx, actorID)
	if err != nil {
		return nil, err
	}
	inviteeAPID, err := lookupActorAPID(ctx, tx, invite.InviteeActorID)
	if err != nil {
		return nil, err
	}
	activityID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	doc := activitypub.ActivityDocument("Undo", activityAPID, actorAPID, invite.APID, inviteeAPID, time.Now().UTC())
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Undo', $3, $4, $5, $6)
	`, activityID, activityAPID, actorID, invite.APID, inviteeAPID, rawDoc); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, actorID, activityID, activityAPID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, invite.InviteeActorID, activityID, activityAPID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE project_invites
		SET status = 'revoked', response_activity_id = $1
		WHERE id = $2
	`, activityID, invite.ID); err != nil {
		return nil, err
	}
	deliveries, err := apdelivery.CreateRowsForInboxes(ctx, tx, activityID, "", recipientInboxes, apdelivery.DefaultMaxRetry)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &MembershipResult{
		ActivityID:       activityID,
		ProjectID:        invite.ProjectID,
		RecipientInboxes: recipientInboxes,
		Deliveries:       deliveries,
	}, nil
}

// writeMemberRemovalActivity stores Remove or Undo activity for member removal.
func (r *PgRepository) writeMemberRemovalActivity(ctx context.Context, tx *sqlx.Tx, projectID, actorID, targetUserID string) (string, error) {
	projectAPID, err := lookupActorAPID(ctx, tx, projectID)
	if err != nil {
		return "", err
	}
	actorAPID, err := lookupActorAPID(ctx, tx, actorID)
	if err != nil {
		return "", err
	}
	targetAPID, err := lookupActorAPID(ctx, tx, targetUserID)
	if err != nil {
		return "", err
	}

	activityType := "Remove"
	objectAPID := targetAPID
	target := projectAPID
	if actorID == targetUserID {
		activityType = "Undo"
		objectAPID = projectAPID
	}

	activityID, err := activitypub.NewID()
	if err != nil {
		return "", err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	doc := activitypub.ActivityDocument(activityType, activityAPID, actorAPID, objectAPID, target, time.Now().UTC())
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, activityID, activityAPID, activityType, actorID, objectAPID, target, rawDoc); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, actorID, activityID, activityAPID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, projectID, activityID, activityAPID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`, targetUserID, activityID, activityAPID); err != nil {
		return "", err
	}
	return activityID, nil
}

// affectedTicketAssignment identifies a ticket whose assignee projection changed.
type affectedTicketAssignment struct {
	ID   string `db:"id"`
	APID string `db:"ap_id"`
}

// removeTicketAssigneeForProjectMember clears assignments for a removed member.
func removeTicketAssigneeForProjectMember(ctx context.Context, tx *sqlx.Tx, projectID, actorID string) ([]affectedTicketAssignment, error) {
	affected := make([]affectedTicketAssignment, 0)
	if err := tx.SelectContext(ctx, &affected, `
		SELECT ticket.id::text, ticket.ap_id
		FROM tickets ticket
		JOIN ticket_assignees assignee ON assignee.ticket_id = ticket.id
		WHERE ticket.project_id = $1
			AND assignee.actor_id = $2
		ORDER BY ticket.created_at ASC, ticket.id ASC
	`, projectID, actorID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM ticket_assignees assignee
		USING tickets ticket
		WHERE ticket.id = assignee.ticket_id
			AND ticket.project_id = $1
			AND assignee.actor_id = $2
	`, projectID, actorID); err != nil {
		return nil, err
	}
	return affected, nil
}

// updateTicketAssignedToDocument rewrites forge:assignedTo in a ticket JSON-LD snapshot.
func updateTicketAssignedToDocument(ctx context.Context, tx *sqlx.Tx, ticketID, ticketAPID string) error {
	var rawDocument []byte
	if err := tx.GetContext(ctx, &rawDocument, `
		SELECT document
		FROM ap_objects
		WHERE ap_id = $1
		FOR UPDATE
	`, ticketAPID); err != nil {
		return err
	}

	var document map[string]any
	if err := json.Unmarshal(rawDocument, &document); err != nil {
		return err
	}

	assigneeAPIDs := make([]string, 0)
	if err := tx.SelectContext(ctx, &assigneeAPIDs, `
		SELECT actor.ap_id
		FROM ticket_assignees assignee
		JOIN actors actor ON actor.id = assignee.actor_id
		WHERE assignee.ticket_id = $1
		ORDER BY assignee.created_at ASC, actor.ap_id ASC
	`, ticketID); err != nil {
		return err
	}
	if len(assigneeAPIDs) == 0 {
		delete(document, "forge:assignedTo")
	} else {
		document["forge:assignedTo"] = assigneeAPIDs
	}

	updatedDocument, err := json.Marshal(document)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE ap_objects
		SET document = $2,
			is_deleted = false
		WHERE ap_id = $1
	`, ticketAPID, updatedDocument)
	return err
}

// tombstoneObject replaces a stored ActivityPub object with a Tombstone document.
func tombstoneObject(ctx context.Context, q sqlx.ExecerContext, apID string, formerType string) error {
	rawDoc, err := json.Marshal(activitypub.TombstoneDocument(apID, formerType, time.Now().UTC()))
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `
		UPDATE ap_objects
		SET object_type = 'Tombstone',
			local_ref_table = NULL,
			local_ref_id = NULL,
			document = $2,
			is_deleted = true
		WHERE ap_id = $1
	`, apID, rawDoc)
	return err
}

// getInviteForUpdate locks and returns a project invite.
func getInviteForUpdate(ctx context.Context, tx *sqlx.Tx, inviteID string) (*ProjectInvite, error) {
	var invite ProjectInvite
	if err := tx.GetContext(ctx, &invite, `
		SELECT
			invite.id::text,
			invite.ap_id,
			invite.project_id::text,
			invite.inviter_actor_id::text,
			invite.invitee_actor_id::text,
			invite.role_id::text,
			project_role.key AS role,
			invite.status,
			invite.created_at,
			invite.updated_at
		FROM project_invites invite
		JOIN project_roles project_role ON project_role.id = invite.role_id
		WHERE invite.id = $1
		FOR UPDATE
	`, inviteID); err != nil {
		return nil, err
	}
	return &invite, nil
}

// isProjectMemberTx reports whether a user is a project member inside a transaction.
func isProjectMemberTx(ctx context.Context, q sqlx.QueryerContext, projectID, userID string) (bool, error) {
	var member bool
	err := sqlx.GetContext(ctx, q, &member, `
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

// hasPendingInviteTx reports whether a user has a pending invite inside a transaction.
func hasPendingInviteTx(ctx context.Context, q sqlx.QueryerContext, projectID, userID string) (bool, error) {
	var pending bool
	err := sqlx.GetContext(ctx, q, &pending, `
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

// lookupActorAPID resolves an actor UUID to its ActivityPub ID.
func lookupActorAPID(ctx context.Context, q sqlx.QueryerContext, actorID string) (string, error) {
	var apID string
	err := sqlx.GetContext(ctx, q, &apID, `SELECT ap_id FROM actors WHERE id = $1`, actorID)
	return apID, err
}

// lookupActivePublicKey returns the active public signing key for an actor.
func lookupActivePublicKey(ctx context.Context, q sqlx.QueryerContext, actorID string) (string, error) {
	var publicKey string
	err := sqlx.GetContext(ctx, q, &publicKey, `
		SELECT public_key_pem FROM actor_keys
		WHERE actor_id = $1 AND active = true
	`, actorID)
	return publicKey, err
}
