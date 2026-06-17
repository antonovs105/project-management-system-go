package notification

import (
	"errors"
	"time"
)

const (
	// TypeTicketAssigned is emitted when a local user is assigned to a ticket.
	TypeTicketAssigned = "ticket.assigned"
)

// ErrRecipientNotLocal reports an assignment target that has no local user inbox.
var ErrRecipientNotLocal = errors.New("notification recipient is not a local user")

// Notification is a local, user-scoped event shown in the UI.
type Notification struct {
	ID        string     `db:"id" json:"id"`
	UserID    string     `db:"user_id" json:"user_id"`
	ActorID   *string    `db:"actor_id" json:"actor_id,omitempty"`
	ProjectID *string    `db:"project_id" json:"project_id,omitempty"`
	TicketID  *string    `db:"ticket_id" json:"ticket_id,omitempty"`
	Type      string     `db:"type" json:"type"`
	Title     string     `db:"title" json:"title"`
	Body      string     `db:"body" json:"body"`
	ReadAt    *time.Time `db:"read_at" json:"read_at,omitempty"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
}

// ListOptions bounds notification list requests.
type ListOptions struct {
	Limit      int
	Offset     int
	UnreadOnly bool
}
