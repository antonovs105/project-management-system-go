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

// GetByID finds single ticket by its id
func (r *Repository) GetByID(ctx context.Context, id int64) (*Ticket, error) {
	var t Ticket
	query := `SELECT * FROM tickets WHERE id = $1`
	err := r.db.GetContext(ctx, &t, query, id)
	return &t, err
}

// Update renews (new synonym!) ticket data in DB
func (r *Repository) Update(ctx context.Context, ticket *Ticket) error {
	query := `
		UPDATE tickets
		SET
			title = :title,
			description = :description,
			status = :status,
			priority = :priority,
			assignee_id = :assignee_id,
			updated_at = now()
		WHERE id = :id`

	result, err := r.db.NamedExecContext(ctx, query, ticket)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("ticket to update not found")
	}

	return nil
}

// Delete removes ticket from DB
func (r *Repository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM tickets WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("ticket to delete not found")
	}

	return nil
}
