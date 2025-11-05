package ticket

import (
	"context"
	"errors"

	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// Create new ticket in DB
func (r *Repository) Create(ctx context.Context, ticket *Ticket) error {
	query := `
		INSERT INTO tickets (title, description, status, priority, project_id, reporter_id, assignee_id)
		VALUES (:title, :description, :status, :priority, :project_id, :reporter_id, :assignee_id)
		RETURNING *`

	rows, err := r.db.NamedQueryContext(ctx, query, ticket)
	if err != nil {
		return err
	}
	defer rows.Close()

	if rows.Next() {
		return rows.StructScan(ticket)
	}
	return errors.New("ticket creation failed: no returning row")
}

// ListByProjectID gets all tickets in a project
func (r *Repository) ListByProjectID(ctx context.Context, projectID int64) ([]Ticket, error) {
	var tickets []Ticket
	query := `SELECT * FROM tickets WHERE project_id = $1 ORDER BY created_at DESC`

	err := r.db.SelectContext(ctx, &tickets, query, projectID)
	if err != nil {
		return nil, err
	}
	return tickets, nil
}
