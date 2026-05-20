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

	QueueFederation = "federation"
	TaskDeliver     = "activitypub:deliver"
	DefaultMaxRetry = 10
)

var (
	ErrDeliveryNotFound  = errors.New("activity delivery not found")
	ErrDeliveryDone      = errors.New("activity delivery already delivered")
	ErrDeliveryExhausted = errors.New("activity delivery attempts exhausted")
	ErrDeliveryConflict  = errors.New("activity delivery conflicts with existing row")
)

type Delivery struct {
	ID             string          `db:"id" json:"id"`
	ActivityID     string          `db:"activity_id" json:"activity_id"`
	ActivityAPID   string          `db:"activity_ap_id" json:"activity_ap_id"`
	ActorID        string          `db:"actor_id" json:"actor_id"`
	ActorAPID      string          `db:"actor_ap_id" json:"actor_ap_id"`
	TargetInboxURL string          `db:"target_inbox_url" json:"target_inbox_url"`
	State          string          `db:"state" json:"state"`
	Attempts       int             `db:"attempts" json:"attempts"`
	MaxAttempts    int             `db:"max_attempts" json:"max_attempts"`
	NextAttemptAt  *time.Time      `db:"next_attempt_at" json:"next_attempt_at,omitempty"`
	LastError      *string         `db:"last_error" json:"last_error,omitempty"`
	DeliveredAt    *time.Time      `db:"delivered_at" json:"delivered_at,omitempty"`
	Document       json.RawMessage `db:"document" json:"-"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at" json:"updated_at"`
}

type TaskPayload struct {
	DeliveryID string `json:"delivery_id"`
}
