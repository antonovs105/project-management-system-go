package remoteinbox

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrBodyTooLarge        = errors.New("inbox activity body too large")
	ErrUnsupportedMedia    = errors.New("unsupported inbox media type")
	ErrUnauthorized        = errors.New("unauthorized inbox activity")
	ErrForbiddenActor      = errors.New("activity actor does not match signature actor")
	ErrTargetNotFound      = errors.New("inbox target actor not found")
	ErrInvalidActivity     = errors.New("invalid inbox activity")
	ErrUnsupportedActivity = errors.New("unsupported inbox activity type")
	ErrActivityConflict    = errors.New("inbox activity conflicts with stored activity")
)

type InboundActivity struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	ActorAPID  string          `json:"actor"`
	ActorID    string          `json:"-"`
	ObjectAPID *string         `json:"object,omitempty"`
	TargetAPID *string         `json:"target,omitempty"`
	Document   json.RawMessage `json:"-"`
}

type AcceptedActivity struct {
	ActivityID           string    `json:"activity_id"`
	ActivityAPID         string    `json:"activity_ap_id"`
	ResponseActivityID   string    `json:"response_activity_id,omitempty"`
	ResponseActivityAPID string    `json:"response_activity_ap_id,omitempty"`
	ReceivedAt           time.Time `json:"received_at"`
	Duplicate            bool      `json:"duplicate"`
}

type FollowResponse struct {
	ActivityID     string
	ActivityAPID   string
	TargetInboxURL string
}
