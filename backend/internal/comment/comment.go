package comment

import (
	"time"

	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
)

// Comment is a local Note object attached to a ticket.
type Comment struct {
	ID        string    `db:"id" json:"id"`
	APID      string    `db:"ap_id" json:"ap_id"`
	TicketID  string    `db:"ticket_id" json:"ticket_id"`
	AuthorID  string    `db:"author_id" json:"author_id"`
	Content   string    `db:"content" json:"content"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// CommentListOptions contains pagination for comment list responses.
type CommentListOptions struct {
	Limit  int
	Offset int
}

// CreateResult carries the ActivityPub side effects produced by comment creation.
type CreateResult struct {
	ActivityID string
	ProjectID  string
	TicketID   string
	Deliveries []apdelivery.QueueCandidate
}

// DeleteResult carries the ActivityPub side effects produced by comment deletion.
type DeleteResult struct {
	ActivityID       string
	ProjectID        string
	RecipientInboxes []string
	Deliveries       []apdelivery.QueueCandidate
}
