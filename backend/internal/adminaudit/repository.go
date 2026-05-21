package adminaudit

import (
	"context"
	"encoding/json"

	"github.com/jmoiron/sqlx"
)

// Repository defines persistence operations for administrative audit events.
type Repository interface {
	InstanceRole(ctx context.Context, userID string) (string, error)
	ListEvents(ctx context.Context, options ListOptions) ([]Event, error)
}

// PgRepository implements Repository using PostgreSQL.
type PgRepository struct {
	db *sqlx.DB
}

// queryRowerContext is the subset of sqlx executors that can insert audit rows.
type queryRowerContext interface {
	QueryRowxContext(ctx context.Context, query string, args ...any) *sqlx.Row
}

// NewRepository creates a PostgreSQL-backed audit repository.
func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

// InstanceRole returns the instance role for a local user.
func (r *PgRepository) InstanceRole(ctx context.Context, userID string) (string, error) {
	var role string
	err := r.db.GetContext(ctx, &role, `SELECT instance_role FROM users WHERE id = $1`, userID)
	return role, err
}

// ListEvents loads audit events matching the given filters.
func (r *PgRepository) ListEvents(ctx context.Context, options ListOptions) ([]Event, error) {
	events := make([]Event, 0)
	if err := r.db.SelectContext(ctx, &events, `
		SELECT id::text, actor_user_id::text, action, target_type, target_id, metadata, created_at
		FROM admin_audit_events
		WHERE ($1 = '' OR action = $1)
			AND (NULLIF($2, '') IS NULL OR actor_user_id = NULLIF($2, '')::uuid)
			AND ($3 = '' OR target_type = $3)
		ORDER BY created_at DESC, id DESC
		LIMIT $4 OFFSET $5
	`, options.Action, options.ActorUserID, options.TargetType, options.Limit, options.Offset); err != nil {
		return nil, err
	}
	return events, nil
}

// InsertEvent appends an administrative audit event using the provided query executor.
func InsertEvent(ctx context.Context, q queryRowerContext, input EventInput) (*Event, error) {
	rawMetadata, err := json.Marshal(input.Metadata)
	if err != nil {
		return nil, err
	}
	if input.Metadata == nil {
		rawMetadata = []byte(`{}`)
	}

	var actorUserID any
	if input.ActorUserID != "" {
		actorUserID = input.ActorUserID
	}

	var event Event
	if err := q.QueryRowxContext(ctx, `
		INSERT INTO admin_audit_events (actor_user_id, action, target_type, target_id, metadata)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, actor_user_id::text, action, target_type, target_id, metadata, created_at
	`, actorUserID, input.Action, input.TargetType, input.TargetID, rawMetadata).StructScan(&event); err != nil {
		return nil, err
	}
	return &event, nil
}
