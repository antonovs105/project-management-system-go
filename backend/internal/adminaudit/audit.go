package adminaudit

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	// ActionUserRoleUpdated records a user role change by an admin.
	ActionUserRoleUpdated = "user.role_updated"
	// ActionFederationDomainBlocked records a domain moderation block.
	ActionFederationDomainBlocked = "federation.domain_blocked"
	// ActionFederationDomainUnblock records removal of a domain moderation block.
	ActionFederationDomainUnblock = "federation.domain_unblocked"
	// ActionFederationDeliveryRetry records a manual federation delivery retry.
	ActionFederationDeliveryRetry = "federation.delivery_retried"

	// TargetTypeUser identifies a local user audit target.
	TargetTypeUser = "user"
	// TargetTypeFederationDomain identifies a domain moderation audit target.
	TargetTypeFederationDomain = "federation_domain"
	// TargetTypeFederationDelivery identifies a federation delivery audit target.
	TargetTypeFederationDelivery = "federation_delivery"

	// roleAdmin is the stored user role required for audit inspection.
	roleAdmin = "admin"
)

var (
	// ErrAdminRequired reports that the current user is not an admin.
	ErrAdminRequired = errors.New("admin role required")
	// ErrInvalidFilter reports malformed audit listing filters.
	ErrInvalidFilter = errors.New("invalid admin audit filter")
)

// Event is a persisted administrative audit record.
type Event struct {
	ID          string          `db:"id" json:"id"`
	ActorUserID *string         `db:"actor_user_id" json:"actor_user_id,omitempty"`
	Action      string          `db:"action" json:"action"`
	TargetType  string          `db:"target_type" json:"target_type"`
	TargetID    string          `db:"target_id" json:"target_id"`
	Metadata    json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt   time.Time       `db:"created_at" json:"created_at"`
}

// EventInput describes a new administrative audit record.
type EventInput struct {
	ActorUserID string
	Action      string
	TargetType  string
	TargetID    string
	Metadata    map[string]any
}

// ListOptions contains filters and pagination for audit event listing.
type ListOptions struct {
	Action      string
	ActorUserID string
	TargetType  string
	Limit       int
	Offset      int
}

// IsAction reports whether value is a supported audit action.
func IsAction(value string) bool {
	switch value {
	case ActionUserRoleUpdated, ActionFederationDomainBlocked, ActionFederationDomainUnblock, ActionFederationDeliveryRetry:
		return true
	default:
		return false
	}
}

// IsTargetType reports whether value is a supported audit target type.
func IsTargetType(value string) bool {
	switch value {
	case TargetTypeUser, TargetTypeFederationDomain, TargetTypeFederationDelivery:
		return true
	default:
		return false
	}
}
