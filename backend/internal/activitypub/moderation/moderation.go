package moderation

import (
	"errors"
	"time"
)

const RoleAdmin = "admin"

var (
	ErrAdminRequired       = errors.New("admin permissions required")
	ErrInvalidDomainBlock  = errors.New("invalid federation domain block")
	ErrDomainBlockNotFound = errors.New("federation domain block not found")
)

type DomainBlock struct {
	ID        string    `db:"id" json:"id"`
	Domain    string    `db:"domain" json:"domain"`
	Reason    string    `db:"reason" json:"reason"`
	CreatedBy *string   `db:"created_by" json:"created_by,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
