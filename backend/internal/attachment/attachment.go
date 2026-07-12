// Package attachment implements permissioned ticket file storage.
package attachment

import (
	"context"
	"errors"
	"io"
	"time"
)

// MaxSizeBytes is the hard per-object attachment limit.
const MaxSizeBytes int64 = 10 << 20

var (
	// ErrForbidden reports a missing project permission.
	ErrForbidden = errors.New("attachment permission denied")
	// ErrNotFound reports a missing attachment or ticket.
	ErrNotFound = errors.New("attachment not found")
	// ErrInvalidFile reports unsafe or malformed file content.
	ErrInvalidFile = errors.New("invalid attachment")
	// ErrInfected reports content rejected by malware scanning.
	ErrInfected = errors.New("attachment failed malware scan")
)

// Attachment is stored metadata for one immutable ticket file.
type Attachment struct {
	ID          string    `db:"id" json:"id"`
	TicketID    string    `db:"ticket_id" json:"ticket_id"`
	UploaderID  string    `db:"uploader_id" json:"uploader_id"`
	ObjectKey   string    `db:"object_key" json:"-"`
	Filename    string    `db:"filename" json:"filename"`
	ContentType string    `db:"content_type" json:"content_type"`
	SizeBytes   int64     `db:"size_bytes" json:"size_bytes"`
	SHA256      string    `db:"sha256" json:"sha256"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

// PermissionChecker verifies project-local authorization.
type PermissionChecker interface {
	HasPermission(ctx context.Context, projectID, userID, permission string) (bool, error)
}

// ObjectStore persists immutable attachment bytes outside PostgreSQL.
type ObjectStore interface {
	Put(ctx context.Context, key string, source io.Reader) (int64, string, error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// MalwareScanner validates file content before persistence.
type MalwareScanner interface {
	Scan(ctx context.Context, source io.Reader) error
}

// StoredObject describes one object visible to storage reconciliation.
type StoredObject struct {
	Key        string
	ModifiedAt time.Time
}

// ObjectLister exposes stored objects to the orphan reconciler.
type ObjectLister interface {
	List(ctx context.Context) ([]StoredObject, error)
}
