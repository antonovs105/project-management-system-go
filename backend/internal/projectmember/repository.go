package projectmember

import (
	"context"

	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// Add user into project
func (r *Repository) Add(ctx context.Context, pm *ProjectMember) error {
	query := `
		INSERT INTO project_members (user_id, project_id, role)
		VALUES (:user_id, :project_id, :role)`
	_, err := r.db.NamedExecContext(ctx, query, pm)
	return err
}

// FindByUserAndProject finds if user in project
func (r *Repository) FindByUserAndProject(ctx context.Context, userID, projectID int64) (*ProjectMember, error) {
	var pm ProjectMember
	query := `SELECT * FROM project_members WHERE user_id = $1 AND project_id = $2`
	err := r.db.GetContext(ctx, &pm, query, userID, projectID)
	return &pm, err
}

// GetUserRoleInProject gets user role in project
func (r *Repository) GetUserRoleInProject(ctx context.Context, userID, projectID int64) (string, error) {
	var role string
	query := `SELECT role FROM project_members WHERE user_id = $1 AND project_id = $2`
	err := r.db.GetContext(ctx, &role, query, userID, projectID)
	return role, err
}
