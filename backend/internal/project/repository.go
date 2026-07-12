package project

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
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
