package user

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// Repository
type Repository struct {
	db *sqlx.DB
}

// Constructor
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{
		db: db,
	}
}

// Add new user
func (r *Repository) CreateUser(ctx context.Context, user *User) error {
	query := `
		INSERT INTO users (username, email, password_hash, role)
		VALUES (:username, :email, :password_hash, :role)
		RETURNING id
	`
	rows, err := r.db.NamedQuery(query, user)
	if err != nil {
		return err
	}
	defer rows.Close()

	if rows.Next() {
		err = rows.Scan(&user.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetUserByEmail finds user by email
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {

	var user User

	query := `SELECT * FROM users WHERE email = $1`

	err := r.db.GetContext(ctx, &user, query, email)
	if err != nil {
		return nil, err
	}

	return &user, nil
}
