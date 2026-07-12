package project

import (
	"context"
	"encoding/json"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/apperror"
	"github.com/jmoiron/sqlx"
)

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
