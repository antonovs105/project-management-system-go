package user

import "time"

type User struct {
	ID            string    `db:"id" json:"id"`
	APID          string    `db:"ap_id" json:"ap_id"`
	Username      string    `db:"username" json:"username"`
	Email         string    `db:"email" json:"email"`
	PasswordHash  string    `db:"password_hash" json:"-"`
	Role          string    `db:"role" json:"role"`
	Handle        string    `db:"handle" json:"handle"`
	Name          string    `db:"name" json:"name"`
	Summary       string    `db:"summary" json:"summary"`
	PublicKeyPEM  string    `db:"public_key_pem" json:"-"`
	PrivateKeyPEM string    `db:"private_key_pem" json:"-"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

type ListUsersOptions struct {
	Role   string
	Query  string
	Limit  int
	Offset int
}
