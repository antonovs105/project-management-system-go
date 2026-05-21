package delivery

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

// Repository defines persistence operations for outbound delivery attempts.
type Repository interface {
	Create(ctx context.Context, activityID string, targetInboxURL string, maxAttempts int) (*Delivery, bool, error)
	CreateWithActor(ctx context.Context, activityID string, actorID string, targetInboxURL string, maxAttempts int) (*Delivery, bool, error)
	StartAttempt(ctx context.Context, deliveryID string) (*Delivery, error)
	MarkDelivered(ctx context.Context, deliveryID string) error
	MarkFailed(ctx context.Context, deliveryID string, message string, details FailureDetails, nextAttemptAt *time.Time) error
}

// RecipientRepository extends Repository with project-recipient lookup operations.
type RecipientRepository interface {
	Repository
	ProjectDeliveries(ctx context.Context, projectID string, userID string, options ProjectDeliveryListOptions) ([]ProjectDelivery, error)
	ProjectDeliverySummary(ctx context.Context, projectID string, userID string) (*ProjectDeliverySummary, error)
	RetryProjectDelivery(ctx context.Context, projectID string, userID string, deliveryID string) (*Delivery, error)
	RemoteProjectFollowerInboxes(ctx context.Context, projectID string) ([]string, error)
	RemoteProjectTicketRecipientInboxes(ctx context.Context, projectID string, ticketID string) ([]string, error)
}

// PgRepository implements delivery repositories using PostgreSQL.
type PgRepository struct {
	db *sqlx.DB
}

// NewRepository creates a PostgreSQL-backed delivery repository.
func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

// NewRecipientRepository creates a PostgreSQL-backed recipient delivery repository.
func NewRecipientRepository(db *sqlx.DB) RecipientRepository {
	return &PgRepository{db: db}
}

// Create creates or loads a delivery for the activity's stored actor.
func (r *PgRepository) Create(ctx context.Context, activityID string, targetInboxURL string, maxAttempts int) (*Delivery, bool, error) {
	return r.create(ctx, activityID, "", targetInboxURL, maxAttempts)
}

// CreateWithActor creates or loads a delivery using an explicit actor.
func (r *PgRepository) CreateWithActor(ctx context.Context, activityID string, actorID string, targetInboxURL string, maxAttempts int) (*Delivery, bool, error) {
	return r.create(ctx, activityID, actorID, targetInboxURL, maxAttempts)
}

// create inserts or loads a delivery row for an activity and target inbox.
func (r *PgRepository) create(ctx context.Context, activityID string, actorID string, targetInboxURL string, maxAttempts int) (*Delivery, bool, error) {
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
		RETURNING id::text
	`, activityID, targetInboxURL, maxAttempts, actorID).Scan(&id)
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

// StartAttempt marks a delivery as processing and increments its attempt count.
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
			last_attempt_at = now(),
			last_failure_kind = '',
			last_status_code = NULL,
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

// MarkDelivered records successful remote delivery.
func (r *PgRepository) MarkDelivered(ctx context.Context, deliveryID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE activity_deliveries
		SET state = $2,
			delivered_at = now(),
			next_attempt_at = NULL,
			last_error = NULL,
			last_failure_kind = '',
			last_status_code = NULL
		WHERE id = $1
	`, deliveryID, StateDelivered)
	return err
}

// MarkFailed records a failed delivery attempt and optional next retry time.
func (r *PgRepository) MarkFailed(ctx context.Context, deliveryID string, message string, details FailureDetails, nextAttemptAt *time.Time) error {
	state := StateFailed
	if nextAttemptAt == nil {
		state = StateDead
	}
	if details.Kind == "" {
		details.Kind = FailureKindUnknown
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE activity_deliveries
		SET state = $2,
			last_error = $3,
			next_attempt_at = $4,
			last_attempt_at = COALESCE(last_attempt_at, now()),
			last_failure_kind = $5,
			last_status_code = $6
		WHERE id = $1
	`, deliveryID, state, message, nextAttemptAt, details.Kind, details.StatusCode)
	return err
}

// ProjectDeliveries returns delivery rows visible in a project.
func (r *PgRepository) ProjectDeliveries(ctx context.Context, projectID string, userID string, options ProjectDeliveryListOptions) ([]ProjectDelivery, error) {
	options, err := NormalizeProjectDeliveryListOptions(options)
	if err != nil {
		return nil, err
	}

	role, err := r.projectDeliveryRole(ctx, r.db, projectID, userID)
	if err != nil {
		return nil, err
	}
	canRetry := canRetryProjectDeliveries(role)

	var deliveries []ProjectDelivery
	err = r.db.SelectContext(ctx, &deliveries, `
		WITH project_scope AS (
			SELECT actor.id, actor.ap_id
			FROM actors actor
			WHERE actor.id = $1
				AND actor.type = 'Group'
				AND actor.is_local = true
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
			d.last_attempt_at,
			d.last_failure_kind,
			d.last_status_code,
			d.delivered_at,
			($3 AND d.state IN ('failed', 'dead')) AS can_retry,
			d.created_at,
			d.updated_at
		FROM activity_deliveries d
		JOIN ap_activities a ON a.id = d.activity_id
		JOIN project_scope project ON project.id = d.project_actor_id
		WHERE ($2 = '' OR d.state = $2)
		ORDER BY d.updated_at DESC, d.created_at DESC
		LIMIT $4
	`, projectID, options.State, canRetry, options.Limit)
	return deliveries, err
}

// ProjectDeliverySummary returns aggregate delivery state counts for a project.
func (r *PgRepository) ProjectDeliverySummary(ctx context.Context, projectID string, userID string) (*ProjectDeliverySummary, error) {
	role, err := r.projectDeliveryRole(ctx, r.db, projectID, userID)
	if err != nil {
		return nil, err
	}

	var summary ProjectDeliverySummary
	err = r.db.GetContext(ctx, &summary, `
		WITH project_scope AS (
			SELECT actor.id, actor.ap_id
			FROM actors actor
			WHERE actor.id = $1
				AND actor.type = 'Group'
				AND actor.is_local = true
		),
		scoped_deliveries AS (
			SELECT d.state
			FROM activity_deliveries d
			JOIN project_scope project ON project.id = d.project_actor_id
		)
		SELECT
			COUNT(*)::int AS total,
			COUNT(*) FILTER (WHERE state = 'pending')::int AS pending,
			COUNT(*) FILTER (WHERE state = 'processing')::int AS processing,
			COUNT(*) FILTER (WHERE state = 'delivered')::int AS delivered,
			COUNT(*) FILTER (WHERE state = 'failed')::int AS failed,
			COUNT(*) FILTER (WHERE state = 'dead')::int AS dead,
			COUNT(*) FILTER (WHERE state IN ('failed', 'dead'))::int AS retryable,
			$2 AS can_retry
		FROM scoped_deliveries
	`, projectID, canRetryProjectDeliveries(role))
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

// RetryProjectDelivery resets a failed delivery for another worker attempt.
func (r *PgRepository) RetryProjectDelivery(ctx context.Context, projectID string, userID string, deliveryID string) (*Delivery, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	role, err := r.projectDeliveryRole(ctx, tx, projectID, userID)
	if err != nil {
		return nil, err
	}
	if !canRetryProjectDeliveries(role) {
		return nil, ErrDeliveryRetryDenied
	}

	var state string
	err = tx.GetContext(ctx, &state, `
		WITH project_scope AS (
			SELECT actor.id
			FROM actors actor
			WHERE actor.id = $1
				AND actor.type = 'Group'
				AND actor.is_local = true
		)
		SELECT d.state
		FROM activity_deliveries d
		JOIN project_scope project ON project.id = d.project_actor_id
		WHERE d.id = $2
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

// RemoteActorAPIDByInboxURL resolves a remote actor by inbox URL.
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

// RemoteProjectFollowerInboxes returns inbox URLs for accepted remote project followers.
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

// RemoteProjectTicketRecipientInboxes returns remote inboxes related to a project ticket.
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

// loadByActivityTarget loads the delivery row for a unique activity and inbox pair.
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

// loadByID loads a delivery row by UUID.
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

// deliverySelect returns the shared delivery projection query.
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
			d.last_attempt_at,
			d.last_failure_kind,
			d.last_status_code,
			d.delivered_at,
			a.document,
			d.created_at,
			d.updated_at
		FROM activity_deliveries d
		JOIN ap_activities a ON a.id = d.activity_id
		JOIN actors actor ON actor.id = d.actor_id
	`
}

// projectDeliveryRole resolves the user's role for delivery inspection and retry.
func (r *PgRepository) projectDeliveryRole(ctx context.Context, q sqlx.QueryerContext, projectID string, userID string) (string, error) {
	role, err := r.projectMemberRole(ctx, q, projectID, userID)
	if err == nil {
		return role, nil
	}
	if !errors.Is(err, ErrProjectAccessDenied) {
		return "", err
	}

	var deletedByUser bool
	if err := sqlx.GetContext(ctx, q, &deletedByUser, `
		SELECT EXISTS(
			SELECT 1
			FROM activity_deliveries delivery
			JOIN ap_activities activity ON activity.id = delivery.activity_id
			JOIN actors project_actor ON project_actor.id = delivery.project_actor_id
			WHERE delivery.project_actor_id = $1
				AND activity.activity_type = 'Delete'
				AND activity.object_ap_id = project_actor.ap_id
				AND activity.actor_id = $2
				AND project_actor.type = 'Group'
				AND project_actor.is_local = true
		)
	`, projectID, userID); err != nil {
		return "", err
	}
	if deletedByUser {
		return "owner", nil
	}
	return "", ErrProjectAccessDenied
}

// projectMemberRole returns the user's project role or access-denied sentinel.
func (r *PgRepository) projectMemberRole(ctx context.Context, q sqlx.QueryerContext, projectID string, userID string) (string, error) {
	var role string
	err := sqlx.GetContext(ctx, q, &role, `
		SELECT role
		FROM project_members
		WHERE project_id = $1 AND user_id = $2
	`, projectID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrProjectAccessDenied
		}
		return "", err
	}
	return role, nil
}

// canRetryProjectDeliveries reports whether a project role may retry deliveries.
func canRetryProjectDeliveries(role string) bool {
	return role == "owner" || role == "manager"
}
