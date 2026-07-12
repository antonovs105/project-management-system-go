package label

import (
	"context"
	"database/sql"
	"errors"

	"github.com/antonovs105/project-management-system-go/internal/apperror"
	"github.com/jmoiron/sqlx"
)

// Repository persists project labels.
type Repository interface {
	List(ctx context.Context, projectID string) ([]Label, error)
	Create(ctx context.Context, item *Label) error
	Delete(ctx context.Context, projectID, labelID string) (bool, error)
}

// PgRepository implements Repository with PostgreSQL.
type PgRepository struct{ db *sqlx.DB }

// NewRepository creates a PostgreSQL label repository.
func NewRepository(db *sqlx.DB) Repository { return &PgRepository{db: db} }

// List returns labels in stable name order.
func (r *PgRepository) List(ctx context.Context, projectID string) ([]Label, error) {
	items := make([]Label, 0)
	err := r.db.SelectContext(ctx, &items, `
		SELECT id::text, project_id::text, name, color, created_at
		FROM project_labels WHERE project_id = $1 ORDER BY lower(name), id
	`, projectID)
	return items, err
}

// Create inserts a label and returns its generated fields.
func (r *PgRepository) Create(ctx context.Context, item *Label) error {
	err := r.db.QueryRowxContext(ctx, `
		INSERT INTO project_labels (project_id, name, color)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
		RETURNING id::text, created_at
	`, item.ProjectID, item.Name, item.Color).Scan(&item.ID, &item.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return apperror.New(apperror.ErrConflict, "label name already exists")
	}
	return err
}

// Delete removes one label owned by a project.
func (r *PgRepository) Delete(ctx context.Context, projectID, labelID string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM project_labels WHERE id = $1 AND project_id = $2`, labelID, projectID)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}
