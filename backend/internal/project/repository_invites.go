package project

import (
	"context"
	"encoding/json"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/apperror"
)

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
