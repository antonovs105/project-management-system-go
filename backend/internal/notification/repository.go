package notification

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Repository defines local notification persistence.
type Repository interface {
	Create(ctx context.Context, notification *Notification) error
	ListByUserID(ctx context.Context, userID string, options ListOptions) ([]Notification, error)
	MarkRead(ctx context.Context, userID, notificationID string) (*Notification, error)
	MarkAllRead(ctx context.Context, userID string) error
}

// PgRepository implements Repository with PostgreSQL.
type PgRepository struct {
	db *sqlx.DB
}

// NewRepository creates a PostgreSQL-backed notification repository.
func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

// Create stores a notification.
func (r *PgRepository) Create(ctx context.Context, notification *Notification) error {
	if notification.ID == "" {
		notification.ID = uuid.NewString()
	}
	if err := r.db.QueryRowxContext(ctx, `
		INSERT INTO notifications (
			id, user_id, actor_id, project_id, ticket_id, type, title, body
		)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8
		WHERE EXISTS (SELECT 1 FROM users WHERE id = $2)
		RETURNING created_at
	`,
		notification.ID,
		notification.UserID,
		notification.ActorID,
		notification.ProjectID,
		notification.TicketID,
		notification.Type,
		notification.Title,
		notification.Body,
	).Scan(&notification.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRecipientNotLocal
		}
		return err
	}
	return nil
}

// ListByUserID returns recent notifications for one local user.
func (r *PgRepository) ListByUserID(ctx context.Context, userID string, options ListOptions) ([]Notification, error) {
	notifications := make([]Notification, 0)
	query := `
		SELECT
			id::text,
			user_id::text,
			actor_id::text,
			project_id::text,
			ticket_id::text,
			type,
			title,
			body,
			read_at,
			created_at
		FROM notifications
		WHERE user_id = $1
			AND ($2 = false OR read_at IS NULL)
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4
	`
	if err := r.db.SelectContext(ctx, &notifications, query, userID, options.UnreadOnly, options.Limit, options.Offset); err != nil {
		return nil, err
	}
	return notifications, nil
}

// MarkRead marks one notification read and returns the updated row.
func (r *PgRepository) MarkRead(ctx context.Context, userID, notificationID string) (*Notification, error) {
	var notification Notification
	if err := r.db.GetContext(ctx, &notification, `
		UPDATE notifications
		SET read_at = COALESCE(read_at, now())
		WHERE id = $1 AND user_id = $2
		RETURNING
			id::text,
			user_id::text,
			actor_id::text,
			project_id::text,
			ticket_id::text,
			type,
			title,
			body,
			read_at,
			created_at
	`, notificationID, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &notification, nil
}

// MarkAllRead marks all unread notifications for one user as read.
func (r *PgRepository) MarkAllRead(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE notifications
		SET read_at = COALESCE(read_at, now())
		WHERE user_id = $1 AND read_at IS NULL
	`, userID)
	return err
}
