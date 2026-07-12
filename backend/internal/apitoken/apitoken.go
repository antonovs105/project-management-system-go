// Package apitoken manages hashed, scoped credentials for non-browser clients.
package apitoken

import "time"

const (
	// ScopeProjectsRead permits read-only project and federation API requests.
	ScopeProjectsRead = "projects:read"
	// ScopeProjectsWrite permits project and federation mutations.
	ScopeProjectsWrite = "projects:write"
	// ScopeAccountRead permits self-service account reads.
	ScopeAccountRead = "account:read"
	// ScopeAccountWrite permits self-service account mutations.
	ScopeAccountWrite = "account:write"
	// ScopeTokensManage permits API-token lifecycle operations.
	ScopeTokensManage = "tokens:manage"
	// ScopeAdmin permits instance-administration endpoints for an admin or owner.
	ScopeAdmin = "admin"
)

// Token is the persisted public representation of an API credential.
type Token struct {
	ID         string     `db:"id" json:"id"`
	UserID     string     `db:"user_id" json:"user_id"`
	Name       string     `db:"name" json:"name"`
	Prefix     string     `db:"token_prefix" json:"prefix"`
	Scopes     []string   `db:"scopes" json:"scopes"`
	ExpiresAt  *time.Time `db:"expires_at" json:"expires_at"`
	LastUsedAt *time.Time `db:"last_used_at" json:"last_used_at"`
	RevokedAt  *time.Time `db:"revoked_at" json:"revoked_at"`
	CreatedAt  time.Time  `db:"created_at" json:"created_at"`
}

// CreatedToken returns plaintext only once alongside persisted metadata.
type CreatedToken struct {
	Token
	Secret string `json:"token"`
}

// CreateRequest contains a token name, scopes, and optional expiry.
type CreateRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
}
