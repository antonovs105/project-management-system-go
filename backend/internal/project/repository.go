package project

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

// Create makes new project in DB
func (r *Repository) Create(ctx context.Context, project *Project) error {
	query := `
		INSERT INTO projects (name, description, owner_id)
		VALUES (:name, :description, :owner_id)
		RETURNING *`

	rows, err := r.db.NamedQueryContext(ctx, query, project)
	if err != nil {
		return err
	}

	defer rows.Close()

	if rows.Next() {
		err = rows.StructScan(project)
		if err != nil {
			return err
		}
	} else {
		return errors.New("project creation failed: no returning row")
	}

	return nil
}

// GetByID finds project by its I=id
func (r *Repository) GetByID(ctx context.Context, id int64) (*Project, error) {
	var p Project
	query := `SELECT * FROM projects WHERE id = $1`
	err := r.db.GetContext(ctx, &p, query, id)
	return &p, err
}

// ListByOwnerID finds all prijects created by user
func (r *Repository) ListByOwnerID(ctx context.Context, ownerID int64) ([]Project, error) {
	var projects []Project

	query := `SELECT * FROM projects WHERE owner_id = $1 ORDER BY created_at DESC`

	err := r.db.SelectContext(ctx, &projects, query, ownerID)
	if err != nil {
		return nil, err
	}

	return projects, nil
}
