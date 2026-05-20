package delivery

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type Repository interface {
	Create(ctx context.Context, activityID string, targetInboxURL string, maxAttempts int) (*Delivery, bool, error)
	StartAttempt(ctx context.Context, deliveryID string) (*Delivery, error)
	MarkDelivered(ctx context.Context, deliveryID string) error
	MarkFailed(ctx context.Context, deliveryID string, message string, nextAttemptAt *time.Time) error
}

type RecipientRepository interface {
	Repository
	ProjectDeliveries(ctx context.Context, projectID string, userID string) ([]ProjectDelivery, error)
	RetryProjectDelivery(ctx context.Context, projectID string, userID string, deliveryID string) (*Delivery, error)
	RemoteProjectFollowerInboxes(ctx context.Context, projectID string) ([]string, error)
	RemoteProjectTicketRecipientInboxes(ctx context.Context, projectID string, ticketID string) ([]string, error)
}

type PgRepository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

func NewRecipientRepository(db *sqlx.DB) RecipientRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) Create(ctx context.Context, activityID string, targetInboxURL string, maxAttempts int) (*Delivery, bool, error) {
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxRetry
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()

	var id string
	err = tx.QueryRowxContext(ctx, `
		INSERT INTO activity_deliveries (
			activity_id, activity_ap_id, actor_id, target_inbox_url, max_attempts
		)
		SELECT id, ap_id, actor_id, $2, $3
		FROM ap_activities
		WHERE id = $1
		ON CONFLICT (activity_id, target_inbox_url) DO NOTHING
		RETURNING id::text
	`, activityID, targetInboxURL, maxAttempts).Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}

	created := id != ""
	delivery, err := loadByActivityTarget(ctx, tx, activityID, targetInboxURL)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return delivery, created, nil
}

func (r *PgRepository) StartAttempt(ctx context.Context, deliveryID string) (*Delivery, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var state string
	var attempts int
	var maxAttempts int
	if err := tx.QueryRowxContext(ctx, `
		SELECT state, attempts, max_attempts
		FROM activity_deliveries
		WHERE id = $1
		FOR UPDATE
	`, deliveryID).Scan(&state, &attempts, &maxAttempts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeliveryNotFound
		}
		return nil, err
	}

	if state == StateDelivered {
		return nil, ErrDeliveryDone
	}
	if state == StateDead || attempts >= maxAttempts {
		return nil, ErrDeliveryExhausted
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE activity_deliveries
		SET state = $2,
			attempts = attempts + 1,
			last_error = NULL,
			next_attempt_at = NULL
		WHERE id = $1
	`, deliveryID, StateProcessing); err != nil {
		return nil, err
	}

	delivery, err := loadByID(ctx, tx, deliveryID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return delivery, nil
}

func (r *PgRepository) MarkDelivered(ctx context.Context, deliveryID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE activity_deliveries
		SET state = $2,
			delivered_at = now(),
			next_attempt_at = NULL,
			last_error = NULL
		WHERE id = $1
	`, deliveryID, StateDelivered)
	return err
}

func (r *PgRepository) MarkFailed(ctx context.Context, deliveryID string, message string, nextAttemptAt *time.Time) error {
	state := StateFailed
	if nextAttemptAt == nil {
		state = StateDead
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE activity_deliveries
		SET state = $2,
			last_error = $3,
			next_attempt_at = $4
		WHERE id = $1
	`, deliveryID, state, message, nextAttemptAt)
	return err
}

func (r *PgRepository) ProjectDeliveries(ctx context.Context, projectID string, userID string) ([]ProjectDelivery, error) {
	var hasAccess bool
	if err := r.db.GetContext(ctx, &hasAccess, `
		SELECT EXISTS(
			SELECT 1
			FROM project_members
			WHERE project_id = $1 AND user_id = $2
		)
	`, projectID, userID); err != nil {
		return nil, err
	}
	if !hasAccess {
		return nil, ErrProjectAccessDenied
	}

	var deliveries []ProjectDelivery
	err := r.db.SelectContext(ctx, &deliveries, `
		WITH project_scope AS (
			SELECT project.id, actor.ap_id
			FROM projects project
			JOIN actors actor ON actor.id = project.id
			WHERE project.id = $1
		)
		SELECT
			d.id::text,
			a.ap_id AS activity_ap_id,
			a.activity_type,
			a.object_ap_id,
			a.target_ap_id,
			d.target_inbox_url,
			d.state,
			d.attempts,
			d.max_attempts,
			d.next_attempt_at,
			d.last_error,
			d.delivered_at,
			d.created_at,
			d.updated_at
		FROM activity_deliveries d
		JOIN ap_activities a ON a.id = d.activity_id
		JOIN project_scope project ON true
		WHERE
			a.object_ap_id = project.ap_id
			OR a.target_ap_id = project.ap_id
			OR EXISTS (
				SELECT 1
				FROM ap_objects object
				JOIN tickets ticket ON ticket.id = object.local_ref_id
				WHERE object.ap_id = a.object_ap_id
					AND object.local_ref_table = 'tickets'
					AND ticket.project_id = project.id
			)
			OR EXISTS (
				SELECT 1
				FROM ap_objects target
				JOIN tickets ticket ON ticket.id = target.local_ref_id
				WHERE target.ap_id = a.target_ap_id
					AND target.local_ref_table = 'tickets'
					AND ticket.project_id = project.id
			)
			OR EXISTS (
				SELECT 1
				FROM ap_objects object
				JOIN comments comment ON comment.id = object.local_ref_id
				JOIN tickets ticket ON ticket.id = comment.ticket_id
				WHERE object.ap_id = a.object_ap_id
					AND object.local_ref_table = 'comments'
					AND ticket.project_id = project.id
			)
			OR EXISTS (
				SELECT 1
				FROM ap_objects target
				JOIN comments comment ON comment.id = target.local_ref_id
				JOIN tickets ticket ON ticket.id = comment.ticket_id
				WHERE target.ap_id = a.target_ap_id
					AND target.local_ref_table = 'comments'
					AND ticket.project_id = project.id
			)
		ORDER BY d.updated_at DESC, d.created_at DESC
		LIMIT 100
	`, projectID)
	return deliveries, err
}

func (r *PgRepository) RetryProjectDelivery(ctx context.Context, projectID string, userID string, deliveryID string) (*Delivery, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var role string
	err = tx.GetContext(ctx, &role, `
		SELECT role
		FROM project_members
		WHERE project_id = $1 AND user_id = $2
	`, projectID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrProjectAccessDenied
		}
		return nil, err
	}
	if role != "owner" && role != "manager" {
		return nil, ErrDeliveryRetryDenied
	}

	var state string
	err = tx.GetContext(ctx, &state, `
		WITH project_scope AS (
			SELECT project.id, actor.ap_id
			FROM projects project
			JOIN actors actor ON actor.id = project.id
			WHERE project.id = $1
		)
		SELECT d.state
		FROM activity_deliveries d
		JOIN ap_activities a ON a.id = d.activity_id
		JOIN project_scope project ON true
		WHERE d.id = $2 AND (
			a.object_ap_id = project.ap_id
			OR a.target_ap_id = project.ap_id
			OR EXISTS (
				SELECT 1
				FROM ap_objects object
				JOIN tickets ticket ON ticket.id = object.local_ref_id
				WHERE object.ap_id = a.object_ap_id
					AND object.local_ref_table = 'tickets'
					AND ticket.project_id = project.id
			)
			OR EXISTS (
				SELECT 1
				FROM ap_objects target
				JOIN tickets ticket ON ticket.id = target.local_ref_id
				WHERE target.ap_id = a.target_ap_id
					AND target.local_ref_table = 'tickets'
					AND ticket.project_id = project.id
			)
			OR EXISTS (
				SELECT 1
				FROM ap_objects object
				JOIN comments comment ON comment.id = object.local_ref_id
				JOIN tickets ticket ON ticket.id = comment.ticket_id
				WHERE object.ap_id = a.object_ap_id
					AND object.local_ref_table = 'comments'
					AND ticket.project_id = project.id
			)
			OR EXISTS (
				SELECT 1
				FROM ap_objects target
				JOIN comments comment ON comment.id = target.local_ref_id
				JOIN tickets ticket ON ticket.id = comment.ticket_id
				WHERE target.ap_id = a.target_ap_id
					AND target.local_ref_table = 'comments'
					AND ticket.project_id = project.id
			)
		)
		FOR UPDATE OF d
	`, projectID, deliveryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeliveryNotFound
		}
		return nil, err
	}

	switch state {
	case StateFailed, StateDead:
	case StateDelivered:
		return nil, ErrDeliveryDone
	default:
		return nil, ErrDeliveryRetryUnavailable
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE activity_deliveries
		SET state = $2,
			attempts = 0,
			max_attempts = $3,
			next_attempt_at = NULL,
			last_error = NULL,
			delivered_at = NULL
		WHERE id = $1
	`, deliveryID, StatePending, DefaultMaxRetry); err != nil {
		return nil, err
	}

	delivery, err := loadByID(ctx, tx, deliveryID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return delivery, nil
}

func (r *PgRepository) RemoteActorAPIDByInboxURL(ctx context.Context, inboxURL string) (string, error) {
	var apID string
	err := r.db.GetContext(ctx, &apID, `
		SELECT ap_id
		FROM actors
		WHERE inbox_url = $1
			AND is_local = false
		ORDER BY updated_at ASC, ap_id ASC
		LIMIT 1
	`, inboxURL)
	return apID, err
}

func (r *PgRepository) RemoteProjectFollowerInboxes(ctx context.Context, projectID string) ([]string, error) {
	var inboxes []string
	err := r.db.SelectContext(ctx, &inboxes, `
		SELECT DISTINCT follower.inbox_url
		FROM actor_follows f
		JOIN actors follower ON follower.id = f.follower_actor_id
		WHERE f.followed_actor_id = $1
			AND f.state = 'accepted'
			AND follower.is_local = false
			AND follower.inbox_url <> ''
		ORDER BY follower.inbox_url ASC
	`, projectID)
	return inboxes, err
}

func (r *PgRepository) RemoteProjectTicketRecipientInboxes(ctx context.Context, projectID string, ticketID string) ([]string, error) {
	var inboxes []string
	err := r.db.SelectContext(ctx, &inboxes, `
		WITH recipients AS (
			SELECT follower.inbox_url
			FROM actor_follows f
			JOIN actors follower ON follower.id = f.follower_actor_id
			WHERE f.followed_actor_id = $1
				AND f.state = 'accepted'
				AND follower.is_local = false
				AND follower.inbox_url <> ''

			UNION

			SELECT reporter.inbox_url
			FROM tickets ticket
			JOIN actors reporter ON reporter.id = ticket.reporter_id
			WHERE ticket.project_id = $1
				AND ticket.id = $2
				AND reporter.is_local = false
				AND reporter.inbox_url <> ''

			UNION

			SELECT assignee.inbox_url
			FROM tickets ticket
			JOIN ticket_assignees ta ON ta.ticket_id = ticket.id
			JOIN actors assignee ON assignee.id = ta.actor_id
			WHERE ticket.project_id = $1
				AND ticket.id = $2
				AND assignee.is_local = false
				AND assignee.inbox_url <> ''

			UNION

			SELECT author.inbox_url
			FROM comments comment
			JOIN tickets ticket ON ticket.id = comment.ticket_id
			JOIN actors author ON author.id = comment.author_id
			WHERE ticket.project_id = $1
				AND ticket.id = $2
				AND author.is_local = false
				AND author.inbox_url <> ''
		)
		SELECT DISTINCT inbox_url
		FROM recipients
		ORDER BY inbox_url ASC
	`, projectID, ticketID)
	return inboxes, err
}

func loadByActivityTarget(ctx context.Context, q sqlx.QueryerContext, activityID string, targetInboxURL string) (*Delivery, error) {
	var delivery Delivery
	err := sqlx.GetContext(ctx, q, &delivery, deliverySelect()+`
		WHERE d.activity_id = $1 AND d.target_inbox_url = $2
	`, activityID, targetInboxURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeliveryNotFound
		}
		return nil, err
	}
	return &delivery, nil
}

func loadByID(ctx context.Context, q sqlx.QueryerContext, deliveryID string) (*Delivery, error) {
	var delivery Delivery
	err := sqlx.GetContext(ctx, q, &delivery, deliverySelect()+`
		WHERE d.id = $1
	`, deliveryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeliveryNotFound
		}
		return nil, err
	}
	return &delivery, nil
}

func deliverySelect() string {
	return `
		SELECT
			d.id::text,
			d.activity_id::text,
			d.activity_ap_id,
			d.actor_id::text,
			actor.ap_id AS actor_ap_id,
			d.target_inbox_url,
			d.state,
			d.attempts,
			d.max_attempts,
			d.next_attempt_at,
			d.last_error,
			d.delivered_at,
			a.document,
			d.created_at,
			d.updated_at
		FROM activity_deliveries d
		JOIN ap_activities a ON a.id = d.activity_id
		JOIN actors actor ON actor.id = d.actor_id
	`
}
