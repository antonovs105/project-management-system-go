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

type PgRepository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) Repository {
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
	if attempts >= maxAttempts {
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
	_, err := r.db.ExecContext(ctx, `
		UPDATE activity_deliveries
		SET state = $2,
			last_error = $3,
			next_attempt_at = $4
		WHERE id = $1
	`, deliveryID, StateFailed, message, nextAttemptAt)
	return err
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
