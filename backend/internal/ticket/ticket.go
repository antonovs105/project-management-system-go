package ticket

import "time"

// Ticket is a ForgeFed-style issue object owned by a project.
type Ticket struct {
	ID          string    `db:"id" json:"id"`
	APID        string    `db:"ap_id" json:"ap_id"`
	Title       string    `db:"title" json:"title"`
	Description string    `db:"description" json:"description"`
	Status      string    `db:"status" json:"status"`
	Priority    string    `db:"priority" json:"priority"`
	Type        string    `db:"type" json:"type"`
	Rank        string    `db:"rank" json:"rank"`
	ParentID    *string   `db:"parent_id" json:"parent_id"`
	ProjectID   string    `db:"project_id" json:"project_id"`
	ReporterID  string    `db:"reporter_id" json:"reporter_id"`
	AssigneeID  *string   `db:"assignee_id" json:"assignee_id"`
	IsResolved  bool      `db:"is_resolved" json:"is_resolved"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// TicketLink is a directed relationship between two tickets.
type TicketLink struct {
	ID        string    `db:"id" json:"id"`
	SourceID  string    `db:"source_id" json:"source_id"`
	TargetID  string    `db:"target_id" json:"target_id"`
	LinkType  string    `db:"link_type" json:"link_type"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// TicketListOptions contains pagination for ticket list responses.
type TicketListOptions struct {
	Limit      int
	Offset     int
	AssigneeID *string
	Unassigned bool
}

// DeleteResult carries the ActivityPub side effects produced by ticket deletion.
type DeleteResult struct {
	ActivityIDs      []string
	RecipientInboxes []string
}
