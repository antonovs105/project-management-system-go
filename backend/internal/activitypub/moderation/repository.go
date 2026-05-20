package moderation

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

type Repository interface {
	UserRole(ctx context.Context, userID string) (string, error)
	ListDomainBlocks(ctx context.Context) ([]DomainBlock, error)
	UpsertDomainBlock(ctx context.Context, domain, reason, userID string) (*DomainBlock, error)
	DeleteDomainBlock(ctx context.Context, domain string) error
	ListRemoteActors(ctx context.Context, options RemoteActorListOptions) ([]RemoteActorInspection, error)
	ListFederationDeliveries(ctx context.Context, options FederationDeliveryListOptions) ([]FederationDeliveryInspection, error)
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
