package adminaudit

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	ActionUserRoleUpdated         = "user.role_updated"
	ActionFederationDomainBlocked = "federation.domain_blocked"
	ActionFederationDomainUnblock = "federation.domain_unblocked"
	ActionFederationDeliveryRetry = "federation.delivery_retried"

	TargetTypeUser               = "user"
	TargetTypeFederationDomain   = "federation_domain"
	TargetTypeFederationDelivery = "federation_delivery"

	roleAdmin = "admin"
)

var (
	ErrAdminRequired = errors.New("admin role required")
	ErrInvalidFilter = errors.New("invalid admin audit filter")
)

type Event struct {
	ID          string          `db:"id" json:"id"`
	ActorUserID *string         `db:"actor_user_id" json:"actor_user_id,omitempty"`
	Action      string          `db:"action" json:"action"`
	TargetType  string          `db:"target_type" json:"target_type"`
	TargetID    string          `db:"target_id" json:"target_id"`
	Metadata    json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt   time.Time       `db:"created_at" json:"created_at"`
}

type EventInput struct {
	ActorUserID string
	Action      string
	TargetType  string
	TargetID    string
	Metadata    map[string]any
}

type ListOptions struct {
	Action      string
	ActorUserID string
	TargetType  string
	Limit       int
	Offset      int
}

func IsAction(value string) bool {
	switch value {
	case ActionUserRoleUpdated, ActionFederationDomainBlocked, ActionFederationDomainUnblock, ActionFederationDeliveryRetry:
		return true
	default:
		return false
	}
}

func IsTargetType(value string) bool {
	switch value {
	case TargetTypeUser, TargetTypeFederationDomain, TargetTypeFederationDelivery:
		return true
	default:
		return false
	}
}
