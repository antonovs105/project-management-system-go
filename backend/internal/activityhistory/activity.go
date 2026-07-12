// Package activityhistory exposes user-facing project and ticket change history.
package activityhistory

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrForbidden reports a missing project read permission.
var ErrForbidden = errors.New("project read permission required")

// ErrNotFound reports a missing project or ticket archive target.
var ErrNotFound = errors.New("archive target not found")

// ErrVersionConflict reports a stale If-Match entity version.
var ErrVersionConflict = errors.New("entity version conflict")

// Event is an immutable project or ticket state transition.
type Event struct {
	ID          string           `db:"id" json:"id"`
	ProjectID   string           `db:"project_id" json:"project_id"`
	ActorID     *string          `db:"actor_id" json:"actor_id"`
	ActorHandle *string          `db:"actor_handle" json:"actor_handle"`
	EntityType  string           `db:"entity_type" json:"entity_type"`
	EntityID    string           `db:"entity_id" json:"entity_id"`
	Action      string           `db:"action" json:"action"`
	BeforeState *json.RawMessage `db:"before_state" json:"before_state"`
	AfterState  *json.RawMessage `db:"after_state" json:"after_state"`
	CreatedAt   time.Time        `db:"created_at" json:"created_at"`
}

// ArchivedProject is a restorable project summary.
type ArchivedProject struct {
	ID          string    `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	Description string    `db:"description" json:"description"`
	Version     int64     `db:"version" json:"version"`
	ArchivedAt  time.Time `db:"archived_at" json:"archived_at"`
}

// ArchivedTicket is a restorable ticket summary.
type ArchivedTicket struct {
	ID         string    `db:"id" json:"id"`
	ProjectID  string    `db:"project_id" json:"project_id"`
	Title      string    `db:"title" json:"title"`
	Version    int64     `db:"version" json:"version"`
	ArchivedAt time.Time `db:"archived_at" json:"archived_at"`
}

// PermissionChecker verifies project-local permissions.
type PermissionChecker interface {
	HasPermission(ctx context.Context, projectID, userID, permission string) (bool, error)
}

// EventRepository loads activity history pages.
type EventRepository interface {
	List(ctx context.Context, projectID string, limit, offset int) ([]Event, error)
}

// ArchiveRepository persists archive and restore transitions.
type ArchiveRepository interface {
	ListArchivedProjects(ctx context.Context, userID string) ([]ArchivedProject, error)
	ListArchivedTickets(ctx context.Context, projectID string) ([]ArchivedTicket, error)
	SetProjectArchived(ctx context.Context, projectID, actorID string, expectedVersion int64, archived bool) error
	SetTicketArchived(ctx context.Context, ticketID, actorID string, expectedVersion int64, archived bool) error
	TicketProjectID(ctx context.Context, ticketID string) (string, error)
}
