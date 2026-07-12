package user

import "time"

// User is a local ActivityPub Person account used for authentication and app access.
type User struct {
	ID            string    `db:"id" json:"id"`
	APID          string    `db:"ap_id" json:"ap_id"`
	Username      string    `db:"username" json:"username"`
	Email         string    `db:"email" json:"email"`
	EmailVerified bool      `db:"email_verified" json:"email_verified"`
	PasswordHash  string    `db:"password_hash" json:"-"`
	InstanceRole  string    `db:"instance_role" json:"instance_role"`
	Handle        string    `db:"handle" json:"handle"`
	Name          string    `db:"name" json:"name"`
	Summary       string    `db:"summary" json:"summary"`
	PublicKeyPEM  string    `db:"public_key_pem" json:"-"`
	PrivateKeyPEM string    `db:"private_key_pem" json:"-"`
	TokenVersion  int       `db:"token_version" json:"-"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

// OAuthIdentity links a provider account to a local user.
type OAuthIdentity struct {
	ID              string    `db:"id" json:"id"`
	UserID          string    `db:"user_id" json:"user_id"`
	Provider        string    `db:"provider" json:"provider"`
	ProviderSubject string    `db:"provider_subject" json:"provider_subject"`
	Email           string    `db:"email" json:"email"`
	EmailVerified   bool      `db:"email_verified" json:"email_verified"`
	DisplayName     string    `db:"display_name" json:"display_name"`
	AvatarURL       string    `db:"avatar_url" json:"avatar_url"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time `db:"updated_at" json:"updated_at"`
}

// ListUsersOptions contains admin filters and pagination for local user listing.
type ListUsersOptions struct {
	InstanceRole string
	Query        string
	Limit        int
	Offset       int
}
