package outboundwebhook

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// ErrNotFound reports a missing project webhook or delivery.
var ErrNotFound = errors.New("outbound webhook not found")

// Repository persists webhook configuration and durable deliveries.
type Repository struct{ db *sqlx.DB }

// NewRepository creates an outbound webhook repository.
func NewRepository(db *sqlx.DB) *Repository { return &Repository{db: db} }

// Create stores a project webhook.
func (r *Repository) Create(ctx context.Context, value *Webhook, secretCipher string) error {
	var row struct {
		ID        string         `db:"id"`
		ProjectID string         `db:"project_id"`
		CreatedBy string         `db:"created_by"`
		Name      string         `db:"name"`
		TargetURL string         `db:"target_url"`
		Events    pq.StringArray `db:"events"`
		Active    bool           `db:"active"`
		CreatedAt time.Time      `db:"created_at"`
		UpdatedAt time.Time      `db:"updated_at"`
	}
	err := r.db.GetContext(ctx, &row, `
		INSERT INTO project_webhooks (project_id, created_by, name, target_url, secret_ciphertext, events)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, project_id::text, created_by::text, name, target_url, events, active, created_at, updated_at
	`, value.ProjectID, value.CreatedBy, value.Name, value.TargetURL, secretCipher, pq.Array(value.Events))
	if err != nil {
		return err
	}
	*value = Webhook{ID: row.ID, ProjectID: row.ProjectID, CreatedBy: row.CreatedBy, Name: row.Name, TargetURL: row.TargetURL, Events: []string(row.Events), Active: row.Active, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	return nil
}

// List returns project webhook metadata.
func (r *Repository) List(ctx context.Context, projectID string) ([]Webhook, error) {
	rows := make([]struct {
		ID        string         `db:"id"`
		ProjectID string         `db:"project_id"`
		CreatedBy string         `db:"created_by"`
		Name      string         `db:"name"`
		TargetURL string         `db:"target_url"`
		Events    pq.StringArray `db:"events"`
		Active    bool           `db:"active"`
		CreatedAt time.Time      `db:"created_at"`
		UpdatedAt time.Time      `db:"updated_at"`
	}, 0)
	if err := r.db.SelectContext(ctx, &rows, `SELECT id::text, project_id::text, created_by::text, name, target_url, events, active, created_at, updated_at FROM project_webhooks WHERE project_id = $1 ORDER BY created_at DESC`, projectID); err != nil {
		return nil, err
	}
	values := make([]Webhook, 0, len(rows))
	for _, row := range rows {
		values = append(values, Webhook{ID: row.ID, ProjectID: row.ProjectID, CreatedBy: row.CreatedBy, Name: row.Name, TargetURL: row.TargetURL, Events: []string(row.Events), Active: row.Active, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
	}
	return values, nil
}

// Delete removes a project webhook and its deliveries.
func (r *Repository) Delete(ctx context.Context, projectID, webhookID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM project_webhooks WHERE id = $1 AND project_id = $2`, webhookID, projectID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// EnqueueActivityEvents materializes new matching activity events exactly once.
func (r *Repository) EnqueueActivityEvents(ctx context.Context) (int, error) {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO project_webhook_deliveries (webhook_id, activity_event_id, event_type, payload)
		SELECT webhook.id, event.id, event.entity_type || '.' || event.action,
			jsonb_build_object(
				'id', event.id,
				'event', event.entity_type || '.' || event.action,
				'project_id', event.project_id,
				'actor_id', event.actor_id,
				'entity_type', event.entity_type,
				'entity_id', event.entity_id,
				'before', event.before_state,
				'after', event.after_state,
				'created_at', event.created_at
			)
		FROM project_webhooks webhook
		JOIN project_activity_events event ON event.project_id = webhook.project_id
		WHERE webhook.active
			AND event.created_at >= webhook.created_at
			AND event.entity_type || '.' || event.action = ANY(webhook.events)
		ON CONFLICT (webhook_id, activity_event_id) DO NOTHING
	`)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	return int(affected), err
}

// Claim leases one due delivery using row locking.
func (r *Repository) Claim(ctx context.Context, now time.Time) (*Delivery, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var value Delivery
	err = tx.GetContext(ctx, &value, `
		SELECT delivery.id::text, delivery.webhook_id::text, webhook.name AS webhook_name,
			webhook.target_url, webhook.secret_ciphertext, delivery.event_type, delivery.payload,
			delivery.status, delivery.attempts, delivery.max_attempts, delivery.next_attempt_at,
			delivery.last_error, delivery.last_status_code, delivery.delivered_at,
			delivery.created_at, delivery.updated_at
		FROM project_webhook_deliveries delivery
		JOIN project_webhooks webhook ON webhook.id = delivery.webhook_id
		WHERE webhook.active AND delivery.status IN ('pending', 'failed') AND delivery.next_attempt_at <= $1
		ORDER BY delivery.next_attempt_at, delivery.created_at
		FOR UPDATE OF delivery SKIP LOCKED
		LIMIT 1
	`, now)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE project_webhook_deliveries SET status = 'processing', attempts = attempts + 1, updated_at = $2 WHERE id = $1`, value.ID, now); err != nil {
		return nil, err
	}
	value.Status = "processing"
	value.Attempts++
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &value, nil
}

// Complete marks a delivery successful.
func (r *Repository) Complete(ctx context.Context, deliveryID string, statusCode int, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE project_webhook_deliveries SET status = 'delivered', delivered_at = $2, last_status_code = $3, last_error = '', updated_at = $2 WHERE id = $1`, deliveryID, now, statusCode)
	return err
}

// Fail records an attempt and schedules retry or terminal dead state.
func (r *Repository) Fail(ctx context.Context, delivery *Delivery, statusCode *int, failure string, next time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE project_webhook_deliveries
		SET status = CASE WHEN attempts >= max_attempts THEN 'dead' ELSE 'failed' END,
			next_attempt_at = $2, last_status_code = $3, last_error = left($4, 1000), updated_at = now()
		WHERE id = $1
	`, delivery.ID, next, statusCode, failure)
	return err
}

// ListDeliveries returns recent delivery diagnostics for a project.
func (r *Repository) ListDeliveries(ctx context.Context, projectID string, limit int) ([]Delivery, error) {
	values := make([]Delivery, 0)
	err := r.db.SelectContext(ctx, &values, `
		SELECT delivery.id::text, delivery.webhook_id::text, webhook.name AS webhook_name,
			webhook.target_url, delivery.event_type, delivery.payload, delivery.status,
			delivery.attempts, delivery.max_attempts, delivery.next_attempt_at,
			delivery.last_error, delivery.last_status_code, delivery.delivered_at,
			delivery.created_at, delivery.updated_at
		FROM project_webhook_deliveries delivery
		JOIN project_webhooks webhook ON webhook.id = delivery.webhook_id
		WHERE webhook.project_id = $1
		ORDER BY delivery.created_at DESC LIMIT $2
	`, projectID, limit)
	return values, err
}

// Retry reschedules a failed or dead project delivery.
func (r *Repository) Retry(ctx context.Context, projectID, deliveryID string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE project_webhook_deliveries delivery
		SET status = 'pending', attempts = 0, next_attempt_at = now(), last_error = '', last_status_code = NULL, updated_at = now()
		FROM project_webhooks webhook
		WHERE delivery.id = $1 AND delivery.webhook_id = webhook.id AND webhook.project_id = $2 AND delivery.status IN ('failed', 'dead')
	`, deliveryID, projectID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
