package delivery

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	// StatePending marks a delivery waiting for worker execution.
	StatePending = "pending"
	// StateProcessing marks a delivery currently claimed by a worker.
	StateProcessing = "processing"
	// StateDelivered marks a delivery accepted by the remote inbox.
	StateDelivered = "delivered"
	// StateFailed marks a retryable delivery failure.
	StateFailed = "failed"
	// StateDead marks a delivery that exhausted retries or failed permanently.
	StateDead = "dead"

	// FailureKindHTTP records non-success HTTP responses from a remote inbox.
	FailureKindHTTP = "http"
	// FailureKindNetwork records retryable network or transport failures.
	FailureKindNetwork = "network"
	// FailureKindSigning records local request-signing failures.
	FailureKindSigning = "signing"
	// FailureKindSafety records blocked unsafe target URL failures.
	FailureKindSafety = "safety"
	// FailureKindUnknown records an unclassified delivery failure.
	FailureKindUnknown = "unknown"
	// QueueFederation is the Asynq queue used for federation delivery tasks.
	QueueFederation = "federation"
	// TaskDeliver is the Asynq task type for sending one ActivityPub delivery.
	TaskDeliver = "activitypub:deliver"
	// DefaultMaxRetry is the default maximum delivery attempt count.
	DefaultMaxRetry = 10
	// DefaultProjectDeliveryListLimit is the default page size for project delivery inspection.
	DefaultProjectDeliveryListLimit = 100
	// MaxProjectDeliveryListLimit caps the page size for project delivery inspection.
	MaxProjectDeliveryListLimit = 500
)

var (
	// ErrDeliveryNotFound reports that a delivery row does not exist.
	ErrDeliveryNotFound = errors.New("activity delivery not found")
	// ErrDeliveryDone reports that a delivery was already completed.
	ErrDeliveryDone = errors.New("activity delivery already delivered")
	// ErrDeliveryExhausted reports that a delivery has no retry attempts remaining.
	ErrDeliveryExhausted = errors.New("activity delivery attempts exhausted")
	// ErrDeliveryConflict reports a conflicting delivery row for the same activity and inbox.
	ErrDeliveryConflict = errors.New("activity delivery conflicts with existing row")
	// ErrInvalidDeliveryFilter reports malformed delivery listing filters.
	ErrInvalidDeliveryFilter = errors.New("invalid delivery filter")
	// ErrDeliveryRetryDenied reports that the current user cannot retry deliveries.
	ErrDeliveryRetryDenied = errors.New("insufficient permissions to retry delivery")
	// ErrDeliveryRetryUnavailable reports that the delivery state cannot be retried.
	ErrDeliveryRetryUnavailable = errors.New("activity delivery cannot be retried")
	// ErrProjectAccessDenied reports that the current user cannot inspect the project.
	ErrProjectAccessDenied = errors.New("project not found or access denied")
)

// Delivery is one outbound attempt target for an ActivityPub activity.
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

// ProjectDelivery is the project-scoped API view of an outbound delivery.
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

// ProjectDeliveryListOptions filters project delivery inspection results.
type ProjectDeliveryListOptions struct {
	State string
	Limit int
}

// ProjectDeliverySummary aggregates delivery states for a project.
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

// TaskPayload is the Asynq payload for one delivery task.
type TaskPayload struct {
	DeliveryID string `json:"delivery_id"`
}

// FailureDetails captures structured failure metadata for persistence.
type FailureDetails struct {
	Kind       string
	StatusCode *int
}

// NormalizeProjectDeliveryListOptions validates and defaults delivery list options.
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

// IsDeliveryState reports whether state is a supported delivery state.
func IsDeliveryState(state string) bool {
	switch state {
	case StatePending, StateProcessing, StateDelivered, StateFailed, StateDead:
		return true
	default:
		return false
	}
}

// IsFailureKind reports whether kind is a supported delivery failure category.
func IsFailureKind(kind string) bool {
	switch kind {
	case FailureKindHTTP, FailureKindNetwork, FailureKindSigning, FailureKindSafety, FailureKindUnknown:
		return true
	default:
		return false
	}
}
