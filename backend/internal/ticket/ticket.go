package ticket

import "time"

type Ticket struct {
	ID          string    `db:"id" json:"id"`
	APID        string    `db:"ap_id" json:"ap_id"`
	Title       string    `db:"title" json:"title"`
	Description string    `db:"description" json:"description"`
	Status      string    `db:"status" json:"status"`
	Priority    string    `db:"priority" json:"priority"`
	Type        string    `db:"type" json:"type"`
	ParentID    *string   `db:"parent_id" json:"parent_id"`
	ProjectID   string    `db:"project_id" json:"project_id"`
	ReporterID  string    `db:"reporter_id" json:"reporter_id"`
	AssigneeID  *string   `db:"assignee_id" json:"assignee_id"`
	IsResolved  bool      `db:"is_resolved" json:"is_resolved"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

type TicketLink struct {
	ID        string    `db:"id" json:"id"`
	SourceID  string    `db:"source_id" json:"source_id"`
	TargetID  string    `db:"target_id" json:"target_id"`
	LinkType  string    `db:"link_type" json:"link_type"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
