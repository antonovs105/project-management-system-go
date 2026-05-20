package moderation

import (
	"context"
	"database/sql"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/jmoiron/sqlx"
)

type Repository interface {
	UserRole(ctx context.Context, userID string) (string, error)
	ListDomainBlocks(ctx context.Context) ([]DomainBlock, error)
	UpsertDomainBlock(ctx context.Context, domain, reason, userID string) (*DomainBlock, error)
	DeleteDomainBlock(ctx context.Context, domain string) error
	ListRemoteActors(ctx context.Context, options RemoteActorListOptions) ([]RemoteActorInspection, error)
	ListFederationDeliveries(ctx context.Context, options FederationDeliveryListOptions) ([]FederationDeliveryInspection, error)
	RetryFederationDelivery(ctx context.Context, deliveryID string) (*delivery.Delivery, error)
}

type PgRepository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

func (r *PgRepository) UserRole(ctx context.Context, userID string) (string, error) {
	var role string
	err := r.db.GetContext(ctx, &role, `SELECT role FROM users WHERE id = $1`, userID)
	return role, err
}

func (r *PgRepository) ListDomainBlocks(ctx context.Context) ([]DomainBlock, error) {
	var blocks []DomainBlock
	if err := r.db.SelectContext(ctx, &blocks, `
		SELECT id::text, domain, reason, created_by::text, created_at, updated_at
		FROM federation_domain_blocks
		ORDER BY domain ASC
	`); err != nil {
		return nil, err
	}
	return blocks, nil
}

func (r *PgRepository) UpsertDomainBlock(ctx context.Context, domain, reason, userID string) (*DomainBlock, error) {
	var block DomainBlock
	if err := r.db.GetContext(ctx, &block, `
		INSERT INTO federation_domain_blocks (domain, reason, created_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (domain) DO UPDATE SET
			reason = EXCLUDED.reason,
			created_by = EXCLUDED.created_by,
			updated_at = now()
		RETURNING id::text, domain, reason, created_by::text, created_at, updated_at
	`, domain, reason, userID); err != nil {
		return nil, err
	}
	return &block, nil
}

func (r *PgRepository) DeleteDomainBlock(ctx context.Context, domain string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM federation_domain_blocks WHERE domain = $1`, domain)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PgRepository) ListRemoteActors(ctx context.Context, options RemoteActorListOptions) ([]RemoteActorInspection, error) {
	var actors []RemoteActorInspection
	if err := r.db.SelectContext(ctx, &actors, `
		SELECT
			id::text,
			ap_id,
			type,
			preferred_username,
			handle,
			name,
			summary,
			inbox_url,
			outbox_url,
			followers_url,
			following_url,
			last_fetched_at,
			fetch_error,
			fetch_error_at,
			created_at,
			updated_at
		FROM actors
		WHERE is_local = false
			AND ($1 = false OR fetch_error IS NOT NULL)
		ORDER BY COALESCE(fetch_error_at, last_fetched_at, updated_at) DESC, ap_id ASC
		LIMIT $2
	`, options.FetchErrorOnly, options.Limit); err != nil {
		return nil, err
	}
	return actors, nil
}

func (r *PgRepository) ListFederationDeliveries(ctx context.Context, options FederationDeliveryListOptions) ([]FederationDeliveryInspection, error) {
	var deliveries []FederationDeliveryInspection
	if err := r.db.SelectContext(ctx, &deliveries, `
		SELECT
			d.id::text,
			a.ap_id AS activity_ap_id,
			a.activity_type,
			actor.ap_id AS actor_ap_id,
			d.project_actor_id::text AS project_id,
			project.ap_id AS project_ap_id,
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
			(d.state IN ('failed', 'dead')) AS can_retry,
			d.created_at,
			d.updated_at
		FROM activity_deliveries d
		JOIN ap_activities a ON a.id = d.activity_id
		JOIN actors actor ON actor.id = d.actor_id
		LEFT JOIN actors project ON project.id = d.project_actor_id
		WHERE ($1 = '' OR d.state = $1)
			AND ($2 = '' OR d.last_failure_kind = $2)
		ORDER BY d.updated_at DESC, d.created_at DESC
		LIMIT $3
	`, options.State, options.FailureKind, options.Limit); err != nil {
		return nil, err
	}
	return deliveries, nil
}

func (r *PgRepository) RetryFederationDelivery(ctx context.Context, deliveryID string) (*delivery.Delivery, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var state string
	if err := tx.GetContext(ctx, &state, `
		SELECT state
		FROM activity_deliveries
		WHERE id = $1
		FOR UPDATE
	`, deliveryID); err != nil {
		if err == sql.ErrNoRows {
			return nil, delivery.ErrDeliveryNotFound
		}
		return nil, err
	}

	switch state {
	case delivery.StateFailed, delivery.StateDead:
	case delivery.StateDelivered:
		return nil, delivery.ErrDeliveryDone
	default:
		return nil, delivery.ErrDeliveryRetryUnavailable
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE activity_deliveries
		SET state = $2,
			attempts = 0,
			max_attempts = $3,
			next_attempt_at = NULL,
			last_error = NULL,
			last_attempt_at = NULL,
			last_failure_kind = '',
			last_status_code = NULL,
			delivered_at = NULL
		WHERE id = $1
	`, deliveryID, delivery.StatePending, delivery.DefaultMaxRetry); err != nil {
		return nil, err
	}

	retried, err := loadFederationDelivery(ctx, tx, deliveryID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return retried, nil
}

func loadFederationDelivery(ctx context.Context, q sqlx.QueryerContext, deliveryID string) (*delivery.Delivery, error) {
	var row delivery.Delivery
	if err := sqlx.GetContext(ctx, q, &row, `
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
		WHERE d.id = $1
	`, deliveryID); err != nil {
		if err == sql.ErrNoRows {
			return nil, delivery.ErrDeliveryNotFound
		}
		return nil, err
	}
	return &row, nil
}
