package notification

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// Repository defines local notification persistence.
type Repository interface {
	Create(ctx context.Context, notification *Notification) error
	ListByUserID(ctx context.Context, userID string, options ListOptions) ([]Notification, error)
	MarkRead(ctx context.Context, userID, notificationID string) (*Notification, error)
	MarkAllRead(ctx context.Context, userID string) error
	ListPreferences(ctx context.Context, userID string) ([]Preference, error)
	GetPreference(ctx context.Context, userID, notificationType string) (*Preference, error)
	UpsertPreference(ctx context.Context, userID string, preference Preference) (*Preference, error)
	VerifiedEmail(ctx context.Context, userID string) (string, error)
	ResolveLocalUserIDs(ctx context.Context, usernames []string) ([]string, error)
	ListDueCandidates(ctx context.Context, now time.Time) ([]DueCandidate, error)
	FederationFailureRecipients(ctx context.Context, actorID string) ([]FederationRecipient, error)
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
			id, user_id, actor_id, project_id, ticket_id, type, title, body, dedupe_key, in_app_visible
		)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		WHERE EXISTS (SELECT 1 FROM users WHERE id = $2)
		ON CONFLICT (user_id, type, dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING
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
		notification.DedupeKey,
		notification.InAppVisible,
	).Scan(&notification.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			var local bool
			if lookupErr := r.db.GetContext(ctx, &local, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, notification.UserID); lookupErr != nil {
				return lookupErr
			}
			if local {
				return ErrDuplicate
			}
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
			dedupe_key,
			read_at,
			created_at
		FROM notifications
		WHERE user_id = $1
			AND in_app_visible = true
			AND ($2 = false OR read_at IS NULL)
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4
	`
	if err := r.db.SelectContext(ctx, &notifications, query, userID, options.UnreadOnly, options.Limit, options.Offset); err != nil {
		return nil, err
	}
	return notifications, nil
}

// ListDueCandidates returns due-soon and overdue tickets assigned to local users.
func (r *PgRepository) ListDueCandidates(ctx context.Context, now time.Time) ([]DueCandidate, error) {
	values := make([]DueCandidate, 0)
	err := r.db.SelectContext(ctx, &values, `
		SELECT
			ta.actor_id::text AS user_id,
			t.project_id::text AS project_id,
			t.id::text AS ticket_id,
			t.title,
			CASE WHEN t.due_date < $1 THEN 'ticket.overdue' ELSE 'ticket.due_soon' END AS type,
			t.id::text || ':' || extract(epoch FROM t.due_date)::bigint::text || ':' ||
				CASE WHEN t.due_date < $1 THEN 'overdue' ELSE 'due-soon' END AS dedupe_key
		FROM tickets t
		JOIN ticket_assignees ta ON ta.ticket_id = t.id
		JOIN users u ON u.id = ta.actor_id
		WHERE t.due_date IS NOT NULL
			AND t.archived_at IS NULL
			AND t.status <> 'done'
			AND t.due_date < $1 + interval '24 hours'
		ORDER BY t.due_date, t.id
		LIMIT 1000
	`, now)
	return values, err
}

// FederationFailureRecipients resolves a local user actor or project owner.
func (r *PgRepository) FederationFailureRecipients(ctx context.Context, actorID string) ([]FederationRecipient, error) {
	values := make([]FederationRecipient, 0, 1)
	err := r.db.SelectContext(ctx, &values, `
		SELECT u.id::text AS user_id, NULL::text AS project_id
		FROM users u WHERE u.id = $1
		UNION ALL
		SELECT p.owner_id::text AS user_id, p.id::text AS project_id
		FROM projects p WHERE p.id = $1
	`, actorID)
	return values, err
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
			dedupe_key,
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

// ListPreferences returns explicit user overrides.
func (r *PgRepository) ListPreferences(ctx context.Context, userID string) ([]Preference, error) {
	values := make([]Preference, 0)
	err := r.db.SelectContext(ctx, &values, `
		SELECT type, in_app_enabled, email_enabled, updated_at
		FROM notification_preferences
		WHERE user_id = $1
		ORDER BY type
	`, userID)
	return values, err
}

// GetPreference returns one explicit user override.
func (r *PgRepository) GetPreference(ctx context.Context, userID, notificationType string) (*Preference, error) {
	var value Preference
	err := r.db.GetContext(ctx, &value, `
		SELECT type, in_app_enabled, email_enabled, updated_at
		FROM notification_preferences
		WHERE user_id = $1 AND type = $2
	`, userID, notificationType)
	return &value, err
}

// UpsertPreference stores one user delivery override.
func (r *PgRepository) UpsertPreference(ctx context.Context, userID string, value Preference) (*Preference, error) {
	err := r.db.GetContext(ctx, &value, `
		INSERT INTO notification_preferences (user_id, type, in_app_enabled, email_enabled)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, type) DO UPDATE SET
			in_app_enabled = EXCLUDED.in_app_enabled,
			email_enabled = EXCLUDED.email_enabled,
			updated_at = now()
		RETURNING type, in_app_enabled, email_enabled, updated_at
	`, userID, value.Type, value.InAppEnabled, value.EmailEnabled)
	return &value, err
}

// VerifiedEmail returns a deliverable address for one local verified user.
func (r *PgRepository) VerifiedEmail(ctx context.Context, userID string) (string, error) {
	var email string
	err := r.db.GetContext(ctx, &email, `
		SELECT email FROM users WHERE id = $1 AND email_verified = true
	`, userID)
	return email, err
}

// ResolveLocalUserIDs resolves case-insensitive comment mention usernames.
func (r *PgRepository) ResolveLocalUserIDs(ctx context.Context, usernames []string) ([]string, error) {
	if len(usernames) == 0 {
		return []string{}, nil
	}
	normalized := make([]string, 0, len(usernames))
	for _, username := range usernames {
		normalized = append(normalized, strings.ToLower(strings.TrimSpace(username)))
	}
	values := make([]string, 0)
	err := r.db.SelectContext(ctx, &values, `
		SELECT id::text FROM users WHERE lower(username) = ANY($1)
	`, pq.Array(normalized))
	return values, err
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
