package federation

import (
	"errors"
	"time"

	"github.com/lib/pq"
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
	// ErrRemoteInviteNotFound reports a missing or inaccessible remote project invite.
	ErrRemoteInviteNotFound = errors.New("remote project invite not found")
	// ErrRemoteInviteNotPending reports a remote project invite that has already been resolved.
	ErrRemoteInviteNotPending = errors.New("remote project invite is not pending")
	// ErrRemoteProjectNotFound reports a missing or inaccessible accepted remote project.
	ErrRemoteProjectNotFound = errors.New("remote project not found")
	// ErrRemoteTicketNotFound reports a missing remote ticket resource.
	ErrRemoteTicketNotFound = errors.New("remote ticket not found")
	// ErrRemoteProjectPermissionDenied reports that the accepted remote role cannot perform a requested write.
	ErrRemoteProjectPermissionDenied = errors.New("remote project permission denied")
	// ErrInvalidRemoteTicketInput reports malformed remote ticket input.
	ErrInvalidRemoteTicketInput = errors.New("invalid remote ticket input")
	// ErrRemoteRequestFailed reports a failed signed remote ActivityPub request.
	ErrRemoteRequestFailed = errors.New("remote activitypub request failed")
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

// RemoteProjectInvite is a remote project invitation addressed to the authenticated user.
type RemoteProjectInvite struct {
	ID              string         `db:"id" json:"id"`
	InviteAPID      string         `db:"invite_ap_id" json:"invite_ap_id"`
	ActivityID      string         `db:"activity_id" json:"activity_id"`
	ProjectAPID     string         `db:"project_ap_id" json:"project_ap_id"`
	ProjectName     string         `db:"project_name" json:"project_name"`
	InviterActorID  string         `db:"inviter_actor_id" json:"inviter_actor_id"`
	InviterAPID     string         `db:"inviter_ap_id" json:"inviter_ap_id"`
	InviterHandle   string         `db:"inviter_handle" json:"inviter_handle"`
	InviterName     string         `db:"inviter_name" json:"inviter_name"`
	InviteeActorID  string         `db:"invitee_actor_id" json:"invitee_actor_id"`
	Role            string         `db:"role" json:"role"`
	RolePermissions pq.StringArray `db:"role_permissions" json:"role_permissions"`
	TargetInboxURL  string         `db:"target_inbox_url" json:"target_inbox_url"`
	Status          string         `db:"status" json:"status"`
	CreatedAt       time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at" json:"updated_at"`
	ResolvedAt      *time.Time     `db:"resolved_at" json:"resolved_at,omitempty"`
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

// RemoteProjectInviteResult describes an accepted or rejected remote project invite.
type RemoteProjectInviteResult struct {
	Invite   RemoteProjectInvite `json:"invite"`
	Delivery *FollowDelivery     `json:"delivery,omitempty"`
}

// RemoteProject is an accepted remote project workspace visible to the authenticated user.
type RemoteProject struct {
	ID              string         `db:"id" json:"id"`
	ProjectAPID     string         `db:"project_ap_id" json:"project_ap_id"`
	ProjectName     string         `db:"project_name" json:"project_name"`
	Role            string         `db:"role" json:"role"`
	RolePermissions pq.StringArray `db:"role_permissions" json:"role_permissions"`
	TargetInboxURL  string         `db:"target_inbox_url" json:"target_inbox_url"`
	InviterActorID  string         `db:"inviter_actor_id" json:"inviter_actor_id"`
	InviterAPID     string         `db:"inviter_ap_id" json:"inviter_ap_id"`
	InviterHandle   string         `db:"inviter_handle" json:"inviter_handle"`
	InviterName     string         `db:"inviter_name" json:"inviter_name"`
	RemoteActorID   *string        `db:"remote_actor_id" json:"remote_actor_id,omitempty"`
	RemoteHandle    *string        `db:"remote_handle" json:"remote_handle,omitempty"`
	CreatedAt       time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at" json:"updated_at"`
	ResolvedAt      *time.Time     `db:"resolved_at" json:"resolved_at,omitempty"`
}

// RemoteTicket is a normalized remote ForgeFed ticket for the local UI.
type RemoteTicket struct {
	ID          string    `json:"id"`
	APID        string    `json:"ap_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Priority    string    `json:"priority"`
	Type        string    `json:"type"`
	Rank        string    `json:"rank"`
	ParentID    *string   `json:"parent_id"`
	ProjectID   string    `json:"project_id"`
	ReporterID  string    `json:"reporter_id"`
	AssigneeID  *string   `json:"assignee_id"`
	IsResolved  bool      `json:"is_resolved"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Raw         []byte    `json:"-"`
}

// RemoteTicketRequest creates or updates a remote ticket through ActivityPub.
type RemoteTicketRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Type        string `json:"type"`
}

// RemoteTicketUpdateRequest updates projected fields on a remote ticket.
type RemoteTicketUpdateRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	Priority    *string `json:"priority"`
	Type        *string `json:"type"`
	IsResolved  *bool   `json:"is_resolved"`
}

// RemoteTicketMoveRequest moves a remote ticket to another workflow status.
type RemoteTicketMoveRequest struct {
	Status string `json:"status"`
}

// RemoteTicketWriteResult describes an outbound remote ticket change.
type RemoteTicketWriteResult struct {
	Ticket   *RemoteTicket   `json:"ticket,omitempty"`
	Delivery *FollowDelivery `json:"delivery,omitempty"`
}

// RemoteInviteResponse carries a stored response activity that must be delivered.
type RemoteInviteResponse struct {
	Invite         *RemoteProjectInvite
	ActivityID     string
	TargetInboxURL string
}

// RemoteProjectActivity carries a stored remote project activity that must be delivered.
type RemoteProjectActivity struct {
	ActivityID     string
	TargetInboxURL string
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
