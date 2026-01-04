package ticket

import "time"

type Ticket struct {
	ID          int64     `db:"id" json:"id"`
	Title       string    `db:"title" json:"title"`
	Description string    `db:"description" json:"description"`
	Status      string    `db:"status" json:"status"`
	Priority    string    `db:"priority" json:"priority"`
	Type        string    `db:"type" json:"type"`
	ParentID    *int64    `db:"parent_id" json:"parent_id"`
	ProjectID   int64     `db:"project_id" json:"project_id"`
	ReporterID  int64     `db:"reporter_id" json:"reporter_id"`
	AssigneeID  *int64    `db:"assignee_id" json:"assignee_id"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

type TicketLink struct {
	ID        int64     `db:"id" json:"id"`
	SourceID  int64     `db:"source_id" json:"source_id"`
	TargetID  int64     `db:"target_id" json:"target_id"`
	LinkType  string    `db:"link_type" json:"link_type"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
