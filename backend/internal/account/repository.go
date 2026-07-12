package account

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

// Repository persists account-security state in PostgreSQL.
type Repository struct {
	db *sqlx.DB
}

// NewRepository returns a PostgreSQL account-security repository.
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// FindUserByEmail returns recovery-safe identity details for a normalized email.
func (r *Repository) FindUserByEmail(ctx context.Context, email string) (string, string, bool, error) {
	var row struct {
		ID            string `db:"id"`
		Email         string `db:"email"`
		EmailVerified bool   `db:"email_verified"`
	}
	err := r.db.GetContext(ctx, &row, `
		SELECT id::text, email, email_verified
		FROM users
		WHERE lower(email) = lower($1)
	`, email)
	return row.ID, row.Email, row.EmailVerified, err
}

// UserEmail returns the current email for an authenticated local user.
func (r *Repository) UserEmail(ctx context.Context, userID string) (string, bool, error) {
	var row struct {
		Email         string `db:"email"`
		EmailVerified bool   `db:"email_verified"`
	}
	err := r.db.GetContext(ctx, &row, `SELECT email, email_verified FROM users WHERE id = $1`, userID)
	return row.Email, row.EmailVerified, err
}

// ReplaceToken invalidates prior challenges of one purpose and queues the replacement email atomically.
func (r *Repository) ReplaceToken(ctx context.Context, userID, purpose, tokenHash, recipient, subject, body string, expiresAt time.Time) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE account_tokens
		SET consumed_at = now()
		WHERE user_id = $1 AND purpose = $2 AND consumed_at IS NULL
	`, userID, purpose); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO account_tokens (user_id, purpose, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, userID, purpose, tokenHash, expiresAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO email_outbox (recipient, subject, text_body)
		VALUES ($1, $2, $3)
	`, recipient, subject, body); err != nil {
		return err
	}
	return tx.Commit()
}

// ConsumeEmailVerification consumes a valid challenge and marks the email verified.
func (r *Repository) ConsumeEmailVerification(ctx context.Context, tokenHash string, now time.Time) (string, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var userID string
	err = tx.GetContext(ctx, &userID, `
		UPDATE account_tokens
		SET consumed_at = $2
		WHERE token_hash = $1 AND purpose = 'verify_email'
			AND consumed_at IS NULL AND expires_at > $2
		RETURNING user_id::text
	`, tokenHash, now)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET email_verified = true, updated_at = now() WHERE id = $1`, userID); err != nil {
		return "", err
	}
	if err := insertAuthEvent(ctx, tx, userID, "email.verified", ClientInfo{}, nil); err != nil {
		return "", err
	}
	return userID, tx.Commit()
}

// ConsumePasswordReset validates a challenge, updates the credential, and revokes every session atomically.
func (r *Repository) ConsumePasswordReset(ctx context.Context, tokenHash, passwordHash string, now time.Time, client ClientInfo) (string, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var userID string
	err = tx.GetContext(ctx, &userID, `
		UPDATE account_tokens
		SET consumed_at = $2
		WHERE token_hash = $1 AND purpose = 'reset_password'
			AND consumed_at IS NULL AND expires_at > $2
		RETURNING user_id::text
	`, tokenHash, now)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET password_hash = $2, token_version = token_version + 1, updated_at = now()
		WHERE id = $1
	`, userID, passwordHash); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE user_sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return "", err
	}
	if err := insertAuthEvent(ctx, tx, userID, "password.reset", client, nil); err != nil {
		return "", err
	}
	return userID, tx.Commit()
}

// CreateSession records a browser session and its durable security event.
func (r *Repository) CreateSession(ctx context.Context, sessionID, userID string, client ClientInfo, expiresAt time.Time) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_sessions (id, user_id, user_agent, ip_address, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, sessionID, userID, client.UserAgent, client.IPAddress, expiresAt); err != nil {
		return err
	}
	if err := insertAuthEvent(ctx, tx, userID, "session.created", client, map[string]any{"session_id": sessionID}); err != nil {
		return err
	}
	return tx.Commit()
}

// ValidateSession rejects expired, revoked, or foreign sessions and periodically refreshes last-seen time.
func (r *Repository) ValidateSession(ctx context.Context, sessionID, userID string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE user_sessions
		SET last_seen_at = CASE WHEN last_seen_at < $3 - interval '5 minutes' THEN $3 ELSE last_seen_at END
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL AND expires_at > $3
	`, sessionID, userID, now)
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

// ListSessions returns a bounded newest-first session inventory.
func (r *Repository) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	values := make([]Session, 0)
	err := r.db.SelectContext(ctx, &values, `
		SELECT id::text, user_agent, ip_address, created_at, last_seen_at, expires_at, revoked_at
		FROM user_sessions
		WHERE user_id = $1 AND expires_at > now() - interval '30 days'
		ORDER BY created_at DESC
		LIMIT 100
	`, userID)
	return values, err
}

// RevokeSession revokes one session owned by the authenticated user.
func (r *Repository) RevokeSession(ctx context.Context, userID, sessionID string, client ClientInfo) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE user_sessions SET revoked_at = now()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
	`, sessionID, userID)
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
	if err := insertAuthEvent(ctx, tx, userID, "session.revoked", client, map[string]any{"session_id": sessionID}); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordAuthEvent appends an immutable security event.
func (r *Repository) RecordAuthEvent(ctx context.Context, userID, eventType string, client ClientInfo, metadata map[string]any) error {
	return insertAuthEvent(ctx, r.db, userID, eventType, client, metadata)
}

// ListAuthEvents returns the authenticated user's most recent security events.
func (r *Repository) ListAuthEvents(ctx context.Context, userID string) ([]SecurityEvent, error) {
	values := make([]SecurityEvent, 0)
	if err := r.db.SelectContext(ctx, &values, `
		SELECT id::text, event_type, ip_address, user_agent, metadata, created_at
		FROM auth_events
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 100
	`, userID); err != nil {
		return nil, err
	}
	for index := range values {
		if err := json.Unmarshal(values[index].RawMetadata, &values[index].Metadata); err != nil {
			values[index].Metadata = map[string]any{}
		}
	}
	return values, nil
}

// ClaimEmail atomically leases one deliverable outbox message.
func (r *Repository) ClaimEmail(ctx context.Context, now time.Time) (*EmailMessage, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var message EmailMessage
	err = tx.GetContext(ctx, &message, `
		SELECT id::text, recipient, subject, text_body, attempts
		FROM email_outbox
		WHERE status IN ('pending', 'failed') AND next_attempt_at <= $1 AND attempts < 8
		ORDER BY created_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`, now)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE email_outbox SET status = 'sending', attempts = attempts + 1 WHERE id = $1`, message.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	message.Attempts++
	return &message, nil
}

// CompleteEmail records a successful outbox delivery.
func (r *Repository) CompleteEmail(ctx context.Context, messageID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE email_outbox SET status = 'sent', sent_at = now(), last_error = '' WHERE id = $1`, messageID)
	return err
}

// FailEmail schedules an exponentially delayed retry without losing the message.
func (r *Repository) FailEmail(ctx context.Context, messageID, failure string, attempts int) error {
	delayMinutes := 1 << min(attempts, 6)
	_, err := r.db.ExecContext(ctx, `
		UPDATE email_outbox
		SET status = 'failed', last_error = left($2, 1000), next_attempt_at = now() + ($3 * interval '1 minute')
		WHERE id = $1
	`, messageID, failure, delayMinutes)
	return err
}

// insertAuthEvent writes an event through a database or transaction executor.
func insertAuthEvent(ctx context.Context, executor sqlx.ExtContext, userID, eventType string, client ClientInfo, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	var nullableUser any
	if userID != "" {
		nullableUser = userID
	}
	_, err = executor.ExecContext(ctx, `
		INSERT INTO auth_events (user_id, event_type, ip_address, user_agent, metadata)
		VALUES ($1, $2, $3, $4, $5)
	`, nullableUser, eventType, client.IPAddress, client.UserAgent, raw)
	return err
}

// IsNotFound reports a missing/expired database challenge without exposing SQL details.
func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
