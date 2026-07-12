package activityhistory

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

// Repository loads immutable activity events.
type Repository struct{ db *sqlx.DB }

// NewRepository returns a PostgreSQL activity repository.
func NewRepository(db *sqlx.DB) *Repository { return &Repository{db: db} }

// List returns a bounded newest-first project history page.
func (r *Repository) List(ctx context.Context, projectID string, limit, offset int) ([]Event, error) {
	values := make([]Event, 0)
	err := r.db.SelectContext(ctx, &values, `
		SELECT e.id::text, e.project_id::text, e.actor_id::text,
			a.handle AS actor_handle, e.entity_type, e.entity_id::text,
			e.action, e.before_state, e.after_state, e.created_at
		FROM project_activity_events e
		LEFT JOIN actors a ON a.id = e.actor_id
		WHERE e.project_id = $1
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT $2 OFFSET $3
	`, projectID, limit, offset)
	return values, err
}

// ListArchivedProjects returns restorable projects where the user remains a member.
func (r *Repository) ListArchivedProjects(ctx context.Context, userID string) ([]ArchivedProject, error) {
	values := make([]ArchivedProject, 0)
	err := r.db.SelectContext(ctx, &values, `
		SELECT p.id::text, p.name, p.description, p.version, p.archived_at
		FROM projects p JOIN project_members member ON member.project_id = p.id
		WHERE member.user_id = $1 AND p.archived_at IS NOT NULL
		ORDER BY p.archived_at DESC
	`, userID)
	return values, err
}

// ListArchivedTickets returns restorable tickets for a project.
func (r *Repository) ListArchivedTickets(ctx context.Context, projectID string) ([]ArchivedTicket, error) {
	values := make([]ArchivedTicket, 0)
	err := r.db.SelectContext(ctx, &values, `
		SELECT id::text, project_id::text, title, version, archived_at
		FROM tickets WHERE project_id = $1 AND archived_at IS NOT NULL
		ORDER BY archived_at DESC
	`, projectID)
	return values, err
}

// SetProjectArchived applies an optimistic archive or restore transition.
func (r *Repository) SetProjectArchived(ctx context.Context, projectID, actorID string, expectedVersion int64, archived bool) error {
	return r.setArchived(ctx, "projects", projectID, actorID, expectedVersion, archived)
}

// SetTicketArchived applies an optimistic archive or restore transition.
func (r *Repository) SetTicketArchived(ctx context.Context, ticketID, actorID string, expectedVersion int64, archived bool) error {
	return r.setArchived(ctx, "tickets", ticketID, actorID, expectedVersion, archived)
}

// TicketProjectID resolves the permission scope for a ticket archive operation.
func (r *Repository) TicketProjectID(ctx context.Context, ticketID string) (string, error) {
	var projectID string
	err := r.db.GetContext(ctx, &projectID, `SELECT project_id::text FROM tickets WHERE id = $1`, ticketID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return projectID, err
}

// setArchived performs the shared transaction and trigger actor attribution.
func (r *Repository) setArchived(ctx context.Context, table, entityID, actorID string, expectedVersion int64, archived bool) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT set_config('progo.actor_id', $1, true)`, actorID); err != nil {
		return err
	}
	value := "NULL"
	condition := "archived_at IS NOT NULL"
	if archived {
		value = "now()"
		condition = "archived_at IS NULL"
	}
	query := `UPDATE ` + table + ` SET archived_at = ` + value + ` WHERE id = $1 AND version = $2 AND ` + condition
	result, err := tx.ExecContext(ctx, query, entityID, expectedVersion)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var exists bool
		if err := tx.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE id = $1)`, entityID); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		return ErrVersionConflict
	}
	return tx.Commit()
}
