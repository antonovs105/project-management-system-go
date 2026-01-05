package project

import (
	"context"
	"errors"

	"github.com/jmoiron/sqlx"
)

type Repository interface {
	Create(ctx context.Context, project *Project) error
	GetByID(ctx context.Context, id int64) (*Project, error)
	ListByOwnerID(ctx context.Context, ownerID int64) ([]Project, error)
	Update(ctx context.Context, project *Project) error
	Delete(ctx context.Context, id int64) error
}

type PgRepository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

// Create makes new project in DB
func (r *PgRepository) Create(ctx context.Context, project *Project) error {
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
func (r *PgRepository) GetByID(ctx context.Context, id int64) (*Project, error) {
	var p Project
	query := `SELECT * FROM projects WHERE id = $1`
	err := r.db.GetContext(ctx, &p, query, id)
	return &p, err
}

// ListByOwnerID finds all prijects created by user
func (r *PgRepository) ListByOwnerID(ctx context.Context, ownerID int64) ([]Project, error) {
	var projects []Project

	query := `SELECT * FROM projects WHERE owner_id = $1 ORDER BY created_at DESC`

	err := r.db.SelectContext(ctx, &projects, query, ownerID)
	if err != nil {
		return nil, err
	}

	return projects, nil
}

// Update updates (how unexpectable) project data in DB
func (r *PgRepository) Update(ctx context.Context, project *Project) error {
	query := `
		UPDATE projects 
		SET 
			name = :name,
			description = :description,
			updated_at = now()
		WHERE id = :id`

	result, err := r.db.NamedExecContext(ctx, query, project)
	if err != nil {
		return err
	}

	// Ckecking is it change enything
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("no rows affected, project not found")
	}

	return nil
}

// Delete deletes (wow) project from DB
func (r *PgRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM projects WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	// check is it changed something
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("project to delete not found")
	}

	return nil
}
