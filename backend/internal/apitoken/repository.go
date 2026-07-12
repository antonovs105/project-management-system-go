package apitoken

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// ErrNotFound reports a missing, revoked, or expired API token.
var ErrNotFound = errors.New("api token not found")

// Repository persists API token hashes and lifecycle metadata.
type Repository struct{ db *sqlx.DB }

// NewRepository creates an API token repository.
func NewRepository(db *sqlx.DB) *Repository { return &Repository{db: db} }

// Create stores a token hash and returns its metadata.
func (r *Repository) Create(ctx context.Context, token *Token, hash []byte) error {
	row := struct {
		ID         string         `db:"id"`
		UserID     string         `db:"user_id"`
		Name       string         `db:"name"`
		Prefix     string         `db:"token_prefix"`
		Scopes     pq.StringArray `db:"scopes"`
		ExpiresAt  *time.Time     `db:"expires_at"`
		LastUsedAt *time.Time     `db:"last_used_at"`
		RevokedAt  *time.Time     `db:"revoked_at"`
		CreatedAt  time.Time      `db:"created_at"`
	}{}
	err := r.db.GetContext(ctx, &row, `
		INSERT INTO api_tokens (user_id, name, token_prefix, token_hash, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, user_id::text, name, token_prefix, scopes, expires_at, last_used_at, revoked_at, created_at
	`, token.UserID, token.Name, token.Prefix, hash, pq.Array(token.Scopes), token.ExpiresAt)
	if err != nil {
		return err
	}
	*token = Token{ID: row.ID, UserID: row.UserID, Name: row.Name, Prefix: row.Prefix, Scopes: []string(row.Scopes), ExpiresAt: row.ExpiresAt, LastUsedAt: row.LastUsedAt, RevokedAt: row.RevokedAt, CreatedAt: row.CreatedAt}
	return nil
}

// List returns all current and historical token metadata for a user.
func (r *Repository) List(ctx context.Context, userID string) ([]Token, error) {
	rows := make([]struct {
		ID         string         `db:"id"`
		UserID     string         `db:"user_id"`
		Name       string         `db:"name"`
		Prefix     string         `db:"token_prefix"`
		Scopes     pq.StringArray `db:"scopes"`
		ExpiresAt  *time.Time     `db:"expires_at"`
		LastUsedAt *time.Time     `db:"last_used_at"`
		RevokedAt  *time.Time     `db:"revoked_at"`
		CreatedAt  time.Time      `db:"created_at"`
	}, 0)
	if err := r.db.SelectContext(ctx, &rows, `
		SELECT id::text, user_id::text, name, token_prefix, scopes, expires_at, last_used_at, revoked_at, created_at
		FROM api_tokens WHERE user_id = $1 ORDER BY created_at DESC
	`, userID); err != nil {
		return nil, err
	}
	values := make([]Token, 0, len(rows))
	for _, row := range rows {
		values = append(values, Token{ID: row.ID, UserID: row.UserID, Name: row.Name, Prefix: row.Prefix, Scopes: []string(row.Scopes), ExpiresAt: row.ExpiresAt, LastUsedAt: row.LastUsedAt, RevokedAt: row.RevokedAt, CreatedAt: row.CreatedAt})
	}
	return values, nil
}

// Revoke atomically invalidates a token owned by userID.
func (r *Repository) Revoke(ctx context.Context, userID, tokenID string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE api_tokens SET revoked_at = now() WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`, tokenID, userID)
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

// Authenticate resolves an active credential hash and coarsely rate-limits last-use writes.
func (r *Repository) Authenticate(ctx context.Context, hash []byte, now time.Time) (string, []string, error) {
	var row struct {
		ID     string         `db:"id"`
		UserID string         `db:"user_id"`
		Scopes pq.StringArray `db:"scopes"`
	}
	err := r.db.GetContext(ctx, &row, `
		SELECT id::text, user_id::text, scopes
		FROM api_tokens
		WHERE token_hash = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > $2)
	`, hash, now)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, ErrNotFound
	}
	if err != nil {
		return "", nil, err
	}
	_, _ = r.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = $2 WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < $2 - interval '5 minutes')`, row.ID, now)
	return row.UserID, []string(row.Scopes), nil
}
