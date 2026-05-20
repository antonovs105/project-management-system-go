package project

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/jmoiron/sqlx"
)

type Repository interface {
	Create(ctx context.Context, project *Project) error
	GetByID(ctx context.Context, id string) (*Project, error)
	ListByOwnerID(ctx context.Context, ownerID string) ([]Project, error)
	GetUserRole(ctx context.Context, userID, projectID string) (string, error)
	Update(ctx context.Context, project *Project) error
	Delete(ctx context.Context, id string) error
	CreateInvite(ctx context.Context, invite *ProjectInvite) error
	AcceptInvite(ctx context.Context, inviteID, userID string) error
}

type PgRepository struct {
	db  *sqlx.DB
	cfg activitypub.Config
}

func NewRepository(db *sqlx.DB, cfg activitypub.Config) Repository {
	return &PgRepository{db: db, cfg: cfg}
}

func (r *PgRepository) Create(ctx context.Context, project *Project) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

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

	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO actor_keys (actor_id, key_id, public_key_pem, private_key_pem)
		VALUES (:actor_id, :key_id, :public_key_pem, :private_key_pem)
	`, map[string]any{
		"actor_id":        project.ID,
		"key_id":          activitypub.KeyID(project.APID),
		"public_key_pem":  project.PublicKeyPEM,
		"private_key_pem": project.PrivateKeyPEM,
	}); err != nil {
		return err
	}

	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO project_members (user_id, project_id, role)
		VALUES (:user_id, :project_id, 'owner')
	`, map[string]any{"user_id": project.OwnerID, "project_id": project.ID}); err != nil {
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
			p.created_at,
			p.updated_at
		FROM projects p
		JOIN actors a ON a.id = p.id
		WHERE p.id = $1
	`
	err := r.db.GetContext(ctx, &p, query, id)
	return &p, err
}

func (r *PgRepository) ListByOwnerID(ctx context.Context, ownerID string) ([]Project, error) {
	var projects []Project
	query := `
		SELECT
			p.id::text,
			a.ap_id,
			p.name,
			p.description,
			p.owner_id::text,
			a.handle,
			p.created_at,
			p.updated_at
		FROM projects p
		JOIN actors a ON a.id = p.id
		JOIN project_members pm ON pm.project_id = p.id
		WHERE pm.user_id = $1
		ORDER BY p.created_at DESC
	`
	if err := r.db.SelectContext(ctx, &projects, query, ownerID); err != nil {
		return nil, err
	}
	return projects, nil
}

func (r *PgRepository) GetUserRole(ctx context.Context, userID, projectID string) (string, error) {
	var role string
	err := r.db.GetContext(ctx, &role, `
		SELECT role
		FROM project_members
		WHERE user_id = $1 AND project_id = $2
	`, userID, projectID)
	return role, err
}

func (r *PgRepository) Update(ctx context.Context, project *Project) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.NamedExecContext(ctx, `
		UPDATE projects
		SET name = :name, description = :description
		WHERE id = :id
	`, project)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("no rows affected, project not found")
	}

	if _, err := tx.NamedExecContext(ctx, `
		UPDATE actors
		SET name = :name, summary = :description
		WHERE id = :id
	`, project); err != nil {
		return err
	}

	publicKey, _ := lookupActivePublicKey(ctx, tx, project.ID)
	doc := activitypub.ProjectActorDocument(project.APID, project.Name, project.Description, publicKey)
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE ap_objects
		SET document = $1, object_type = 'Group'
		WHERE ap_id = $2
	`, rawDoc, project.APID); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PgRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM actors WHERE id = $1`, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("project to delete not found")
	}
	return nil
}

func (r *PgRepository) CreateInvite(ctx context.Context, invite *ProjectInvite) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	projectAPID, err := lookupActorAPID(ctx, tx, invite.ProjectID)
	if err != nil {
		return err
	}
	inviterAPID, err := lookupActorAPID(ctx, tx, invite.InviterActorID)
	if err != nil {
		return err
	}
	inviteeAPID, err := lookupActorAPID(ctx, tx, invite.InviteeActorID)
	if err != nil {
		return err
	}

	activityID, err := activitypub.NewID()
	if err != nil {
		return err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	invite.APID = activityAPID

	object := map[string]any{
		"type":   "Group",
		"id":     projectAPID,
		"target": inviteeAPID,
		"role":   invite.Role,
	}
	doc := activitypub.ActivityDocument("Invite", activityAPID, inviterAPID, object, inviteeAPID, time.Now().UTC())
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Invite', $3, $4, $5, $6)
	`, activityID, activityAPID, invite.InviterActorID, projectAPID, inviteeAPID, rawDoc); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, invite.InviterActorID, activityID, activityAPID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, invite.InviteeActorID, activityID, activityAPID); err != nil {
		return err
	}

	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO project_invites (
			id, ap_id, project_id, inviter_actor_id, invitee_actor_id, role, status, invite_activity_id
		)
		VALUES (
			:id, :ap_id, :project_id, :inviter_actor_id, :invitee_actor_id, :role, 'pending', :invite_activity_id
		)
	`, map[string]any{
		"id":                 invite.ID,
		"ap_id":              invite.APID,
		"project_id":         invite.ProjectID,
		"inviter_actor_id":   invite.InviterActorID,
		"invitee_actor_id":   invite.InviteeActorID,
		"role":               invite.Role,
		"invite_activity_id": activityID,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PgRepository) AcceptInvite(ctx context.Context, inviteID, userID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var invite ProjectInvite
	if err := tx.GetContext(ctx, &invite, `
		SELECT
			id::text,
			ap_id,
			project_id::text,
			inviter_actor_id::text,
			invitee_actor_id::text,
			role,
			status,
			created_at,
			updated_at
		FROM project_invites
		WHERE id = $1
	`, inviteID); err != nil {
		return err
	}
	if invite.InviteeActorID != userID {
		return errors.New("invite does not belong to current user")
	}
	if invite.Status != "pending" {
		return errors.New("invite is not pending")
	}

	actorAPID, err := lookupActorAPID(ctx, tx, userID)
	if err != nil {
		return err
	}
	projectAPID, err := lookupActorAPID(ctx, tx, invite.ProjectID)
	if err != nil {
		return err
	}
	activityID, err := activitypub.NewID()
	if err != nil {
		return err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	doc := activitypub.ActivityDocument("Accept", activityAPID, actorAPID, invite.APID, projectAPID, time.Now().UTC())
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Accept', $3, $4, $5, $6)
	`, activityID, activityAPID, userID, invite.APID, projectAPID, rawDoc); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, userID, activityID, activityAPID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, invite.ProjectID, activityID, activityAPID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_members (user_id, project_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, project_id) DO UPDATE SET role = EXCLUDED.role
	`, userID, invite.ProjectID, invite.Role); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
		VALUES ($1, $2, 'accepted')
		ON CONFLICT (follower_actor_id, followed_actor_id)
		DO UPDATE SET state = 'accepted'
	`, userID, invite.ProjectID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE project_invites
		SET status = 'accepted', response_activity_id = $1
		WHERE id = $2
	`, activityID, invite.ID); err != nil {
		return err
	}

	return tx.Commit()
}

func lookupActorAPID(ctx context.Context, q sqlx.QueryerContext, actorID string) (string, error) {
	var apID string
	err := sqlx.GetContext(ctx, q, &apID, `SELECT ap_id FROM actors WHERE id = $1`, actorID)
	return apID, err
}

func lookupActivePublicKey(ctx context.Context, q sqlx.QueryerContext, actorID string) (string, error) {
	var publicKey string
	err := sqlx.GetContext(ctx, q, &publicKey, `
		SELECT public_key_pem FROM actor_keys
		WHERE actor_id = $1 AND active = true
	`, actorID)
	return publicKey, err
}
