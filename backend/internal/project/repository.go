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
	IsProjectMember(ctx context.Context, projectID, userID string) (bool, error)
	HasPendingInvite(ctx context.Context, projectID, userID string) (bool, error)
	Update(ctx context.Context, project *Project, actorID string) (*UpdateResult, error)
	Delete(ctx context.Context, id string, actorID string) (*DeleteResult, error)
	RemoveMember(ctx context.Context, projectID, actorID, targetUserID string) error
	GetInviteByID(ctx context.Context, inviteID string) (*ProjectInvite, error)
	CreateInvite(ctx context.Context, invite *ProjectInvite) error
	AcceptInvite(ctx context.Context, inviteID, userID string) error
	RejectInvite(ctx context.Context, inviteID, userID string) error
	RevokeInvite(ctx context.Context, inviteID, actorID string) error
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

func (r *PgRepository) IsProjectMember(ctx context.Context, projectID, userID string) (bool, error) {
	var member bool
	err := r.db.GetContext(ctx, &member, `
		SELECT EXISTS(
			SELECT 1
			FROM project_members
			WHERE project_id = $1 AND user_id = $2
		)
	`, projectID, userID)
	return member, err
}

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

func (r *PgRepository) Update(ctx context.Context, project *Project, actorID string) (*UpdateResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	recipientInboxes, err := remoteProjectFollowerInboxes(ctx, tx, project.ID)
	if err != nil {
		return nil, err
	}

	result, err := tx.NamedExecContext(ctx, `
		UPDATE projects
		SET name = :name, description = :description
		WHERE id = :id
	`, project)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, errors.New("no rows affected, project not found")
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

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &UpdateResult{
		ActivityID:       activityID,
		ProjectID:        project.ID,
		RecipientInboxes: recipientInboxes,
	}, nil
}

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
		return nil, errors.New("project to delete not found")
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
		return nil, errors.New("project to delete not found")
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &DeleteResult{
		ActivityID:       activityID,
		ProjectID:        stored.ID,
		RecipientInboxes: recipientInboxes,
	}, nil
}

func remoteProjectFollowerInboxes(ctx context.Context, q sqlx.QueryerContext, projectID string) ([]string, error) {
	var inboxes []string
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

func tombstoneProjectTree(ctx context.Context, tx *sqlx.Tx, projectID, projectAPID string) error {
	var commentAPIDs []string
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

	var ticketAPIDs []string
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

func (r *PgRepository) RemoveMember(ctx context.Context, projectID, actorID, targetUserID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var targetRole string
	if err := tx.GetContext(ctx, &targetRole, `
		SELECT role
		FROM project_members
		WHERE project_id = $1 AND user_id = $2
		FOR UPDATE
	`, projectID, targetUserID); err != nil {
		return errors.New("target user is not a project member")
	}

	if targetRole == RoleOwner {
		var owners int
		if err := tx.GetContext(ctx, &owners, `
			SELECT count(*)
			FROM project_members
			WHERE project_id = $1 AND role = 'owner'
		`, projectID); err != nil {
			return err
		}
		if owners <= 1 {
			return errors.New("cannot remove the last project owner")
		}

		var storedOwnerID string
		if err := tx.GetContext(ctx, &storedOwnerID, `SELECT owner_id::text FROM projects WHERE id = $1`, projectID); err != nil {
			return err
		}
		if storedOwnerID == targetUserID {
			var nextOwnerID string
			if err := tx.GetContext(ctx, &nextOwnerID, `
				SELECT user_id::text
				FROM project_members
				WHERE project_id = $1
					AND role = 'owner'
					AND user_id <> $2
				ORDER BY created_at ASC, user_id ASC
				LIMIT 1
			`, projectID, targetUserID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE projects
				SET owner_id = $2
				WHERE id = $1
			`, projectID, nextOwnerID); err != nil {
				return err
			}
		}
	}

	affectedTickets, err := removeTicketAssigneeForProjectMember(ctx, tx, projectID, targetUserID)
	if err != nil {
		return err
	}
	for _, ticket := range affectedTickets {
		if err := updateTicketAssignedToDocument(ctx, tx, ticket.ID, ticket.APID); err != nil {
			return err
		}
	}

	result, err := tx.ExecContext(ctx, `
		DELETE FROM project_members
		WHERE project_id = $1 AND user_id = $2
	`, projectID, targetUserID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("target user is not a project member")
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM actor_follows
		WHERE follower_actor_id = $1 AND followed_actor_id = $2
	`, targetUserID, projectID); err != nil {
		return err
	}

	if err := r.writeMemberRemovalActivity(ctx, tx, projectID, actorID, targetUserID); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PgRepository) GetInviteByID(ctx context.Context, inviteID string) (*ProjectInvite, error) {
	var invite ProjectInvite
	if err := r.db.GetContext(ctx, &invite, `
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
		return nil, err
	}
	return &invite, nil
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

	member, err := isProjectMemberTx(ctx, tx, invite.ProjectID, invite.InviteeActorID)
	if err != nil {
		return err
	}
	if member {
		return errors.New("user is already a project member")
	}
	pending, err := hasPendingInviteTx(ctx, tx, invite.ProjectID, invite.InviteeActorID)
	if err != nil {
		return err
	}
	if pending {
		return errors.New("pending invite already exists")
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
	member, err := isProjectMemberTx(ctx, tx, invite.ProjectID, userID)
	if err != nil {
		return err
	}
	if member {
		return errors.New("user is already a project member")
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

func (r *PgRepository) RejectInvite(ctx context.Context, inviteID, userID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	invite, err := getInviteForUpdate(ctx, tx, inviteID)
	if err != nil {
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
	doc := activitypub.ActivityDocument("Reject", activityAPID, actorAPID, invite.APID, projectAPID, time.Now().UTC())
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Reject', $3, $4, $5, $6)
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
		UPDATE project_invites
		SET status = 'rejected', response_activity_id = $1
		WHERE id = $2
	`, activityID, invite.ID); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PgRepository) RevokeInvite(ctx context.Context, inviteID, actorID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	invite, err := getInviteForUpdate(ctx, tx, inviteID)
	if err != nil {
		return err
	}
	if invite.Status != "pending" {
		return errors.New("invite is not pending")
	}

	actorAPID, err := lookupActorAPID(ctx, tx, actorID)
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
	doc := activitypub.ActivityDocument("Undo", activityAPID, actorAPID, invite.APID, inviteeAPID, time.Now().UTC())
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Undo', $3, $4, $5, $6)
	`, activityID, activityAPID, actorID, invite.APID, inviteeAPID, rawDoc); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, actorID, activityID, activityAPID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, invite.InviteeActorID, activityID, activityAPID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE project_invites
		SET status = 'revoked', response_activity_id = $1
		WHERE id = $2
	`, activityID, invite.ID); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PgRepository) writeMemberRemovalActivity(ctx context.Context, tx *sqlx.Tx, projectID, actorID, targetUserID string) error {
	projectAPID, err := lookupActorAPID(ctx, tx, projectID)
	if err != nil {
		return err
	}
	actorAPID, err := lookupActorAPID(ctx, tx, actorID)
	if err != nil {
		return err
	}
	targetAPID, err := lookupActorAPID(ctx, tx, targetUserID)
	if err != nil {
		return err
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
		return err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	doc := activitypub.ActivityDocument(activityType, activityAPID, actorAPID, objectAPID, target, time.Now().UTC())
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, activityID, activityAPID, activityType, actorID, objectAPID, target, rawDoc); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, actorID, activityID, activityAPID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, projectID, activityID, activityAPID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`, targetUserID, activityID, activityAPID); err != nil {
		return err
	}
	return nil
}

type affectedTicketAssignment struct {
	ID   string `db:"id"`
	APID string `db:"ap_id"`
}

func removeTicketAssigneeForProjectMember(ctx context.Context, tx *sqlx.Tx, projectID, actorID string) ([]affectedTicketAssignment, error) {
	var affected []affectedTicketAssignment
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

	var assigneeAPIDs []string
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

func getInviteForUpdate(ctx context.Context, tx *sqlx.Tx, inviteID string) (*ProjectInvite, error) {
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
		FOR UPDATE
	`, inviteID); err != nil {
		return nil, err
	}
	return &invite, nil
}

func isProjectMemberTx(ctx context.Context, q sqlx.QueryerContext, projectID, userID string) (bool, error) {
	var member bool
	err := sqlx.GetContext(ctx, q, &member, `
		SELECT EXISTS(
			SELECT 1
			FROM project_members
			WHERE project_id = $1 AND user_id = $2
		)
	`, projectID, userID)
	return member, err
}

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
