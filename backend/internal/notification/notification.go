package notification

import (
	"errors"
	"time"
)

const (
	// TypeTicketAssigned is emitted when a local user is assigned to a ticket.
	TypeTicketAssigned = "ticket.assigned"
	// TypeTicketStatusChanged is emitted for participant-visible workflow changes.
	TypeTicketStatusChanged = "ticket.status_changed"
	// TypeTicketDueSoon is emitted once when an assigned ticket approaches its due date.
	TypeTicketDueSoon = "ticket.due_soon"
	// TypeTicketOverdue is emitted once when an assigned ticket becomes overdue.
	TypeTicketOverdue = "ticket.overdue"
	// TypeCommentCreated is emitted for ticket participants when a comment is added.
	TypeCommentCreated = "comment.created"
	// TypeCommentMentioned is emitted when a local username is mentioned in a comment.
	TypeCommentMentioned = "comment.mentioned"
	// TypeProjectInvited is emitted for local project invitees.
	TypeProjectInvited = "project.invited"
	// TypeProjectRoleChanged is emitted when a local member's project role changes.
	TypeProjectRoleChanged = "project.role_changed"
	// TypeFederationDeliveryFailed is emitted when a delivery becomes terminally failed.
	TypeFederationDeliveryFailed = "federation.delivery_failed"
	// TypeSecurityEvent is reserved for user-visible account security events.
	TypeSecurityEvent = "security.event"
)

// SupportedTypes is the stable notification preference catalog.
var SupportedTypes = []string{
	TypeTicketAssigned,
	TypeTicketStatusChanged,
	TypeTicketDueSoon,
	TypeTicketOverdue,
	TypeCommentCreated,
	TypeCommentMentioned,
	TypeProjectInvited,
	TypeProjectRoleChanged,
	TypeFederationDeliveryFailed,
	TypeSecurityEvent,
}

// ErrRecipientNotLocal reports an assignment target that has no local user inbox.
var ErrRecipientNotLocal = errors.New("notification recipient is not a local user")

// ErrDuplicate reports a previously delivered deduplicated notification.
var ErrDuplicate = errors.New("notification already delivered")

// Notification is a local, user-scoped event shown in the UI.
type Notification struct {
	ID           string     `db:"id" json:"id"`
	UserID       string     `db:"user_id" json:"user_id"`
	ActorID      *string    `db:"actor_id" json:"actor_id,omitempty"`
	ProjectID    *string    `db:"project_id" json:"project_id,omitempty"`
	TicketID     *string    `db:"ticket_id" json:"ticket_id,omitempty"`
	Type         string     `db:"type" json:"type"`
	Title        string     `db:"title" json:"title"`
	Body         string     `db:"body" json:"body"`
	DedupeKey    *string    `db:"dedupe_key" json:"-"`
	InAppVisible bool       `db:"in_app_visible" json:"-"`
	ReadAt       *time.Time `db:"read_at" json:"read_at,omitempty"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
}

// DueCandidate is one local assignee notification selected by the due-date dispatcher.
type DueCandidate struct {
	UserID    string `db:"user_id"`
	ProjectID string `db:"project_id"`
	TicketID  string `db:"ticket_id"`
	Title     string `db:"title"`
	Type      string `db:"type"`
	DedupeKey string `db:"dedupe_key"`
}

// FederationRecipient maps a local delivery actor to the user who should be alerted.
type FederationRecipient struct {
	UserID    string  `db:"user_id"`
	ProjectID *string `db:"project_id"`
}

// Preference controls in-app and email delivery for one notification type.
type Preference struct {
	Type         string     `db:"type" json:"type"`
	InAppEnabled bool       `db:"in_app_enabled" json:"in_app_enabled"`
	EmailEnabled bool       `db:"email_enabled" json:"email_enabled"`
	UpdatedAt    *time.Time `db:"updated_at" json:"updated_at,omitempty"`
}

// ListOptions bounds notification list requests.
type ListOptions struct {
	Limit      int
	Offset     int
	UnreadOnly bool
}
