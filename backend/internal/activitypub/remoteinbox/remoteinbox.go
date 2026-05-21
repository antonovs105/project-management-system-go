package remoteinbox

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	// ErrBodyTooLarge reports an inbox payload exceeding the configured limit.
	ErrBodyTooLarge = errors.New("inbox activity body too large")
	// ErrUnsupportedMedia reports an inbox request with an unsupported content type.
	ErrUnsupportedMedia = errors.New("unsupported inbox media type")
	// ErrUnauthorized reports a missing or invalid HTTP signature.
	ErrUnauthorized = errors.New("unauthorized inbox activity")
	// ErrForbiddenActor reports that the activity actor differs from the signed actor.
	ErrForbiddenActor = errors.New("activity actor does not match signature actor")
	// ErrBlockedDomain reports an activity from a blocked remote actor domain.
	ErrBlockedDomain = errors.New("inbox activity actor domain is blocked")
	// ErrTargetNotFound reports that the addressed local inbox actor does not exist.
	ErrTargetNotFound = errors.New("inbox target actor not found")
	// ErrInvalidActivity reports a malformed inbound ActivityStreams document.
	ErrInvalidActivity = errors.New("invalid inbox activity")
	// ErrUnsupportedActivity reports a valid activity type not handled by this server.
	ErrUnsupportedActivity = errors.New("unsupported inbox activity type")
	// ErrActivityConflict reports a duplicate activity with conflicting stored content.
	ErrActivityConflict = errors.New("inbox activity conflicts with stored activity")
)

// InboundActivity is the normalized form of a remote inbox activity.
type InboundActivity struct {
	ID             string            `json:"id"`
	Type           string            `json:"type"`
	ActorAPID      string            `json:"actor"`
	ActorID        string            `json:"-"`
	ObjectAPID     *string           `json:"object,omitempty"`
	ObjectActivity *EmbeddedActivity `json:"-"`
	ObjectNote     *InboundNote      `json:"-"`
	ObjectTicket   *InboundTicket    `json:"-"`
	TargetAPID     *string           `json:"target,omitempty"`
	Document       json.RawMessage   `json:"-"`
}

// EmbeddedActivity represents the object of an Undo or response activity.
type EmbeddedActivity struct {
	ID         string
	Type       string
	ActorAPID  string
	ObjectAPID string
}

// InboundNote is the normalized Note object accepted by project inboxes.
type InboundNote struct {
	ID           string
	AttributedTo string
	InReplyTo    string
	Content      string
	Document     json.RawMessage
}

// InboundTicket is the normalized ForgeFed ticket object accepted by project inboxes.
type InboundTicket struct {
	ID               string
	AttributedTo     string
	Context          string
	Name             string
	HasName          bool
	Content          string
	HasContent       bool
	Status           string
	HasStatus        bool
	Priority         string
	HasPriority      bool
	TicketType       string
	HasTicketType    bool
	IsResolved       bool
	HasIsResolved    bool
	InvalidFieldType bool
	Document         json.RawMessage
}

// AcceptedActivity describes an accepted or deduplicated inbound activity.
type AcceptedActivity struct {
	ActivityID           string    `json:"activity_id"`
	ActivityAPID         string    `json:"activity_ap_id"`
	ResponseActivityID   string    `json:"response_activity_id,omitempty"`
	ResponseActivityAPID string    `json:"response_activity_ap_id,omitempty"`
	ReceivedAt           time.Time `json:"received_at"`
	Duplicate            bool      `json:"duplicate"`
}

// FollowResponse describes the Accept activity emitted for an inbound Follow.
type FollowResponse struct {
	ActivityID     string
	ActivityAPID   string
	TargetInboxURL string
}

// InviteResponseType records whether an inbound invite response accepted or rejected.
type InviteResponseType string

const (
	// InviteResponseAccept marks an inbound invite response as accepted.
	InviteResponseAccept InviteResponseType = "accepted"
	// InviteResponseReject marks an inbound invite response as rejected.
	InviteResponseReject InviteResponseType = "rejected"
)
