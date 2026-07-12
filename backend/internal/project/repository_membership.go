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
	"github.com/jmoiron/sqlx"
)

// UpdateMemberRole changes a collaborator role while preserving a project manager.
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
