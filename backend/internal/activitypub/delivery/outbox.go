package delivery

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jmoiron/sqlx"
)

// CreateRowsForInboxes writes pending delivery rows inside the caller's transaction.
func CreateRowsForInboxes(ctx context.Context, q sqlx.QueryerContext, activityID string, actorID string, inboxes []string, maxAttempts int) ([]QueueCandidate, error) {
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxRetry
	}
	uniqueInboxes := uniqueNonEmptyStrings(inboxes)
	deliveries := make([]QueueCandidate, 0, len(uniqueInboxes))
	for _, inbox := range uniqueInboxes {
		candidate, err := createRowForInbox(ctx, q, activityID, actorID, inbox, maxAttempts)
		if err != nil {
			return nil, err
		}
		if candidate.ID != "" {
			deliveries = append(deliveries, candidate)
		}
	}
	return deliveries, nil
}

// createRowForInbox writes or loads one pending delivery row for one inbox.
func createRowForInbox(ctx context.Context, q sqlx.QueryerContext, activityID string, actorID string, inbox string, maxAttempts int) (QueueCandidate, error) {
	var row struct {
		ID          string `db:"id"`
		MaxAttempts int    `db:"max_attempts"`
		State       string `db:"state"`
	}
	err := sqlx.GetContext(ctx, q, &row, `
		WITH inserted AS (
			INSERT INTO activity_deliveries (
				activity_id, activity_ap_id, actor_id, project_actor_id, target_inbox_url, max_attempts
			)
			SELECT
				activity.id,
				activity.ap_id,
				COALESCE(NULLIF($4, '')::uuid, activity.actor_id),
				COALESCE(
					target_project_actor.id,
					object_project_actor.id,
					activity_project_actor.id,
					object_ticket.project_id,
					target_ticket.project_id,
					object_comment_ticket.project_id,
					target_comment_ticket.project_id,
					object_invite.project_id,
					target_invite.project_id
				),
				$2,
				$3
			FROM ap_activities activity
			LEFT JOIN actors target_project_actor
				ON target_project_actor.ap_id = activity.target_ap_id
				AND target_project_actor.type = 'Group'
				AND target_project_actor.is_local = true
			LEFT JOIN actors object_project_actor
				ON object_project_actor.ap_id = activity.object_ap_id
				AND object_project_actor.type = 'Group'
				AND object_project_actor.is_local = true
			LEFT JOIN actors activity_project_actor
				ON activity_project_actor.id = activity.actor_id
				AND activity_project_actor.type = 'Group'
				AND activity_project_actor.is_local = true
			LEFT JOIN ap_objects object_scope ON object_scope.ap_id = activity.object_ap_id
			LEFT JOIN tickets object_ticket
				ON object_scope.local_ref_table = 'tickets'
				AND object_ticket.id = object_scope.local_ref_id
			LEFT JOIN comments object_comment
				ON object_scope.local_ref_table = 'comments'
				AND object_comment.id = object_scope.local_ref_id
			LEFT JOIN tickets object_comment_ticket ON object_comment_ticket.id = object_comment.ticket_id
			LEFT JOIN ap_objects target_scope ON target_scope.ap_id = activity.target_ap_id
			LEFT JOIN tickets target_ticket
				ON target_scope.local_ref_table = 'tickets'
				AND target_ticket.id = target_scope.local_ref_id
			LEFT JOIN comments target_comment
				ON target_scope.local_ref_table = 'comments'
				AND target_comment.id = target_scope.local_ref_id
			LEFT JOIN tickets target_comment_ticket ON target_comment_ticket.id = target_comment.ticket_id
			LEFT JOIN project_invites object_invite ON object_invite.ap_id = activity.object_ap_id
			LEFT JOIN project_invites target_invite ON target_invite.ap_id = activity.target_ap_id
			WHERE activity.id = $1
			ON CONFLICT (activity_id, target_inbox_url) DO NOTHING
			RETURNING id::text, max_attempts, state
		)
		SELECT id, max_attempts, state FROM inserted
		UNION ALL
		SELECT id::text, max_attempts, state
		FROM activity_deliveries
		WHERE activity_id = $1 AND target_inbox_url = $2
		LIMIT 1
	`, activityID, inbox, maxAttempts, actorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return QueueCandidate{}, ErrDeliveryNotFound
		}
		return QueueCandidate{}, err
	}
	if row.State == StateDelivered || row.State == StateDead {
		return QueueCandidate{}, nil
	}
	return QueueCandidate{ID: row.ID, MaxAttempts: row.MaxAttempts}, nil
}

// uniqueNonEmptyStrings trims empty values and preserves first-seen order.
func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
