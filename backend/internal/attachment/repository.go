package attachment

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

// Repository persists attachment metadata.
type Repository struct{ db *sqlx.DB }

// NewRepository returns a PostgreSQL attachment repository.
func NewRepository(db *sqlx.DB) *Repository { return &Repository{db: db} }

// TicketProjectID resolves an attachment permission scope.
func (r *Repository) TicketProjectID(ctx context.Context, ticketID string) (string, error) {
	var projectID string
	err := r.db.GetContext(ctx, &projectID, `SELECT project_id::text FROM tickets WHERE id = $1 AND archived_at IS NULL`, ticketID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return projectID, err
}

// Create inserts immutable attachment metadata.
func (r *Repository) Create(ctx context.Context, value *Attachment) error {
	return r.db.GetContext(ctx, value, `
		INSERT INTO ticket_attachments (ticket_id, uploader_id, object_key, filename, content_type, size_bytes, sha256)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text, ticket_id::text, uploader_id::text, object_key, filename, content_type, size_bytes, sha256, created_at
	`, value.TicketID, value.UploaderID, value.ObjectKey, value.Filename, value.ContentType, value.SizeBytes, value.SHA256)
}

// List returns ticket attachments in display order.
func (r *Repository) List(ctx context.Context, ticketID string) ([]Attachment, error) {
	values := make([]Attachment, 0)
	err := r.db.SelectContext(ctx, &values, `
		SELECT id::text, ticket_id::text, uploader_id::text, object_key, filename, content_type, size_bytes, sha256, created_at
		FROM ticket_attachments WHERE ticket_id = $1 ORDER BY created_at, id
	`, ticketID)
	return values, err
}

// Get returns one attachment and its project scope.
func (r *Repository) Get(ctx context.Context, attachmentID string) (*Attachment, string, error) {
	var row struct {
		Attachment
		ProjectID string `db:"project_id"`
	}
	err := r.db.GetContext(ctx, &row, `
		SELECT a.id::text, a.ticket_id::text, a.uploader_id::text, a.object_key, a.filename,
			a.content_type, a.size_bytes, a.sha256, a.created_at, t.project_id::text
		FROM ticket_attachments a JOIN tickets t ON t.id = a.ticket_id WHERE a.id = $1
	`, attachmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	return &row.Attachment, row.ProjectID, err
}

// Delete removes one metadata row and returns its object key.
func (r *Repository) Delete(ctx context.Context, attachmentID string) (string, error) {
	var key string
	err := r.db.GetContext(ctx, &key, `DELETE FROM ticket_attachments WHERE id = $1 RETURNING object_key`, attachmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return key, err
}

// ObjectKeys returns the complete set of objects referenced by metadata.
func (r *Repository) ObjectKeys(ctx context.Context) (map[string]struct{}, error) {
	values := make([]string, 0)
	if err := r.db.SelectContext(ctx, &values, `SELECT object_key FROM ticket_attachments`); err != nil {
		return nil, err
	}
	keys := make(map[string]struct{}, len(values))
	for _, value := range values {
		keys[value] = struct{}{}
	}
	return keys, nil
}
