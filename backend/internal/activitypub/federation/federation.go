package federation

import (
	"errors"
	"time"
)

var (
	// ErrInvalidFilter reports malformed personal federation query filters.
	ErrInvalidFilter = errors.New("invalid federation filter")
	// ErrInvalidRemoteResource reports malformed remote federation resources.
	ErrInvalidRemoteResource = errors.New("invalid remote federation resource")
	// ErrRemoteActorUnavailable reports a remote actor that could not be resolved.
	ErrRemoteActorUnavailable = errors.New("remote federation actor unavailable")
	// ErrLocalActorNotFound reports an authenticated user without a local actor.
	ErrLocalActorNotFound = errors.New("local federation actor not found")
)

// InboxActivity is a normalized personal federation inbox item for app UI.
type InboxActivity struct {
	ID            string    `db:"id" json:"id"`
	ActivityAPID  string    `db:"activity_ap_id" json:"activity_ap_id"`
	ActivityType  string    `db:"activity_type" json:"activity_type"`
	ActorID       string    `db:"actor_id" json:"actor_id"`
	ActorAPID     string    `db:"actor_ap_id" json:"actor_ap_id"`
	ActorType     string    `db:"actor_type" json:"actor_type"`
	ActorHandle   string    `db:"actor_handle" json:"actor_handle"`
	ActorName     string    `db:"actor_name" json:"actor_name"`
	ObjectAPID    *string   `db:"object_ap_id" json:"object_ap_id,omitempty"`
	ObjectType    *string   `db:"object_type" json:"object_type,omitempty"`
	ObjectName    *string   `db:"object_name" json:"object_name,omitempty"`
	ObjectContent *string   `db:"object_content" json:"object_content,omitempty"`
	TargetAPID    *string   `db:"target_ap_id" json:"target_ap_id,omitempty"`
	TargetActorID *string   `db:"target_actor_id" json:"target_actor_id,omitempty"`
	TargetType    *string   `db:"target_type" json:"target_type,omitempty"`
	TargetHandle  *string   `db:"target_handle" json:"target_handle,omitempty"`
	TargetName    *string   `db:"target_name" json:"target_name,omitempty"`
	ReceivedAt    time.Time `db:"received_at" json:"received_at"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
}

// RemoteFollow is a remote actor followed by the authenticated user.
type RemoteFollow struct {
	ActorID           string    `db:"actor_id" json:"actor_id"`
	ActorAPID         string    `db:"actor_ap_id" json:"actor_ap_id"`
	ActorType         string    `db:"actor_type" json:"actor_type"`
	PreferredUsername string    `db:"preferred_username" json:"preferred_username"`
	Handle            string    `db:"handle" json:"handle"`
	Name              string    `db:"name" json:"name"`
	Summary           string    `db:"summary" json:"summary"`
	InboxURL          string    `db:"inbox_url" json:"inbox_url"`
	OutboxURL         string    `db:"outbox_url" json:"outbox_url"`
	FollowersURL      *string   `db:"followers_url" json:"followers_url,omitempty"`
	FollowingURL      *string   `db:"following_url" json:"following_url,omitempty"`
	State             string    `db:"state" json:"state"`
	CreatedAt         time.Time `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time `db:"updated_at" json:"updated_at"`
}

// RemoteActor is a user-facing remote ActivityPub actor projection.
type RemoteActor struct {
	ID                string     `json:"id"`
	APID              string     `json:"ap_id"`
	Type              string     `json:"type"`
	PreferredUsername string     `json:"preferred_username"`
	Handle            string     `json:"handle"`
	Name              string     `json:"name"`
	Summary           string     `json:"summary"`
	InboxURL          string     `json:"inbox_url"`
	OutboxURL         string     `json:"outbox_url"`
	FollowersURL      *string    `json:"followers_url,omitempty"`
	FollowingURL      *string    `json:"following_url,omitempty"`
	LastFetchedAt     *time.Time `json:"last_fetched_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// FollowDelivery is the outbound delivery created for a Follow activity.
type FollowDelivery struct {
	ID             string    `json:"id"`
	ActivityAPID   string    `json:"activity_ap_id"`
	TargetInboxURL string    `json:"target_inbox_url"`
	State          string    `json:"state"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// FollowRemoteActorResult describes an outbound Follow request.
type FollowRemoteActorResult struct {
	Follow   RemoteFollow    `json:"follow"`
	Delivery *FollowDelivery `json:"delivery,omitempty"`
	Created  bool            `json:"created"`
}

// LocalActor is the authenticated local ActivityPub actor used for outbound work.
type LocalActor struct {
	ID   string `db:"id"`
	APID string `db:"ap_id"`
}

// ListOptions controls personal federation list pagination.
type ListOptions struct {
	Limit  int
	Offset int
	State  string
}
