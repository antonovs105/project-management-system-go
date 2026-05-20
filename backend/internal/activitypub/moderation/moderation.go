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
	ErrInvalidFilter       = errors.New("invalid federation moderation filter")
)

type DomainBlock struct {
	ID        string    `db:"id" json:"id"`
	Domain    string    `db:"domain" json:"domain"`
	Reason    string    `db:"reason" json:"reason"`
	CreatedBy *string   `db:"created_by" json:"created_by,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

type RemoteActorInspection struct {
	ID                string     `db:"id" json:"id"`
	APID              string     `db:"ap_id" json:"ap_id"`
	Type              string     `db:"type" json:"type"`
	PreferredUsername string     `db:"preferred_username" json:"preferred_username"`
	Handle            string     `db:"handle" json:"handle"`
	Name              string     `db:"name" json:"name"`
	Summary           string     `db:"summary" json:"summary"`
	InboxURL          string     `db:"inbox_url" json:"inbox_url"`
	OutboxURL         string     `db:"outbox_url" json:"outbox_url"`
	FollowersURL      *string    `db:"followers_url" json:"followers_url,omitempty"`
	FollowingURL      *string    `db:"following_url" json:"following_url,omitempty"`
	LastFetchedAt     *time.Time `db:"last_fetched_at" json:"last_fetched_at,omitempty"`
	FetchError        *string    `db:"fetch_error" json:"fetch_error,omitempty"`
	FetchErrorAt      *time.Time `db:"fetch_error_at" json:"fetch_error_at,omitempty"`
	CreatedAt         time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at" json:"updated_at"`
}

type RemoteActorListOptions struct {
	FetchErrorOnly bool
	Limit          int
}

type FederationDeliveryInspection struct {
	ID              string     `db:"id" json:"id"`
	ActivityAPID    string     `db:"activity_ap_id" json:"activity_ap_id"`
	ActivityType    string     `db:"activity_type" json:"activity_type"`
	ActorAPID       string     `db:"actor_ap_id" json:"actor_ap_id"`
	ProjectID       *string    `db:"project_id" json:"project_id,omitempty"`
	ProjectAPID     *string    `db:"project_ap_id" json:"project_ap_id,omitempty"`
	ObjectAPID      *string    `db:"object_ap_id" json:"object_ap_id,omitempty"`
	TargetAPID      *string    `db:"target_ap_id" json:"target_ap_id,omitempty"`
	TargetInboxURL  string     `db:"target_inbox_url" json:"target_inbox_url"`
	State           string     `db:"state" json:"state"`
	Attempts        int        `db:"attempts" json:"attempts"`
	MaxAttempts     int        `db:"max_attempts" json:"max_attempts"`
	NextAttemptAt   *time.Time `db:"next_attempt_at" json:"next_attempt_at,omitempty"`
	LastError       *string    `db:"last_error" json:"last_error,omitempty"`
	LastAttemptAt   *time.Time `db:"last_attempt_at" json:"last_attempt_at,omitempty"`
	LastFailureKind string     `db:"last_failure_kind" json:"last_failure_kind,omitempty"`
	LastStatusCode  *int       `db:"last_status_code" json:"last_status_code,omitempty"`
	DeliveredAt     *time.Time `db:"delivered_at" json:"delivered_at,omitempty"`
	CanRetry        bool       `db:"can_retry" json:"can_retry"`
	CreatedAt       time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at" json:"updated_at"`
}

type FederationDeliveryListOptions struct {
	State       string
	FailureKind string
	Limit       int
}
