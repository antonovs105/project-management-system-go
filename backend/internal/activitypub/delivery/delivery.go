package delivery

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	StatePending    = "pending"
	StateProcessing = "processing"
	StateDelivered  = "delivered"
	StateFailed     = "failed"
	StateDead       = "dead"

	FailureKindHTTP                 = "http"
	FailureKindNetwork              = "network"
	FailureKindSigning              = "signing"
	FailureKindSafety               = "safety"
	FailureKindUnknown              = "unknown"
	QueueFederation                 = "federation"
	TaskDeliver                     = "activitypub:deliver"
	DefaultMaxRetry                 = 10
	DefaultProjectDeliveryListLimit = 100
	MaxProjectDeliveryListLimit     = 500
)

var (
	ErrDeliveryNotFound         = errors.New("activity delivery not found")
	ErrDeliveryDone             = errors.New("activity delivery already delivered")
	ErrDeliveryExhausted        = errors.New("activity delivery attempts exhausted")
	ErrDeliveryConflict         = errors.New("activity delivery conflicts with existing row")
	ErrInvalidDeliveryFilter    = errors.New("invalid delivery filter")
	ErrDeliveryRetryDenied      = errors.New("insufficient permissions to retry delivery")
	ErrDeliveryRetryUnavailable = errors.New("activity delivery cannot be retried")
	ErrProjectAccessDenied      = errors.New("project not found or access denied")
)

type Delivery struct {
	ID              string          `db:"id" json:"id"`
	ActivityID      string          `db:"activity_id" json:"activity_id"`
	ActivityAPID    string          `db:"activity_ap_id" json:"activity_ap_id"`
	ActorID         string          `db:"actor_id" json:"actor_id"`
	ActorAPID       string          `db:"actor_ap_id" json:"actor_ap_id"`
	TargetInboxURL  string          `db:"target_inbox_url" json:"target_inbox_url"`
	State           string          `db:"state" json:"state"`
	Attempts        int             `db:"attempts" json:"attempts"`
	MaxAttempts     int             `db:"max_attempts" json:"max_attempts"`
	NextAttemptAt   *time.Time      `db:"next_attempt_at" json:"next_attempt_at,omitempty"`
	LastError       *string         `db:"last_error" json:"last_error,omitempty"`
	LastAttemptAt   *time.Time      `db:"last_attempt_at" json:"last_attempt_at,omitempty"`
	LastFailureKind string          `db:"last_failure_kind" json:"last_failure_kind,omitempty"`
	LastStatusCode  *int            `db:"last_status_code" json:"last_status_code,omitempty"`
	DeliveredAt     *time.Time      `db:"delivered_at" json:"delivered_at,omitempty"`
	Document        json.RawMessage `db:"document" json:"-"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at" json:"updated_at"`
}

type ProjectDelivery struct {
	ID              string     `db:"id" json:"id"`
	ActivityAPID    string     `db:"activity_ap_id" json:"activity_ap_id"`
	ActivityType    string     `db:"activity_type" json:"activity_type"`
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

type ProjectDeliveryListOptions struct {
	State string
	Limit int
}

type ProjectDeliverySummary struct {
	Total      int  `db:"total" json:"total"`
	Pending    int  `db:"pending" json:"pending"`
	Processing int  `db:"processing" json:"processing"`
	Delivered  int  `db:"delivered" json:"delivered"`
	Failed     int  `db:"failed" json:"failed"`
	Dead       int  `db:"dead" json:"dead"`
	Retryable  int  `db:"retryable" json:"retryable"`
	CanRetry   bool `db:"can_retry" json:"can_retry"`
}

type TaskPayload struct {
	DeliveryID string `json:"delivery_id"`
}

type FailureDetails struct {
	Kind       string
	StatusCode *int
}

func NormalizeProjectDeliveryListOptions(options ProjectDeliveryListOptions) (ProjectDeliveryListOptions, error) {
	if options.State != "" && !IsDeliveryState(options.State) {
		return ProjectDeliveryListOptions{}, ErrInvalidDeliveryFilter
	}
	if options.Limit == 0 {
		options.Limit = DefaultProjectDeliveryListLimit
	}
	if options.Limit < 0 {
		return ProjectDeliveryListOptions{}, ErrInvalidDeliveryFilter
	}
	if options.Limit > MaxProjectDeliveryListLimit {
		options.Limit = MaxProjectDeliveryListLimit
	}
	return options, nil
}

func IsDeliveryState(state string) bool {
	switch state {
	case StatePending, StateProcessing, StateDelivered, StateFailed, StateDead:
		return true
	default:
		return false
	}
}

func IsFailureKind(kind string) bool {
	switch kind {
	case FailureKindHTTP, FailureKindNetwork, FailureKindSigning, FailureKindSafety, FailureKindUnknown:
		return true
	default:
		return false
	}
}
