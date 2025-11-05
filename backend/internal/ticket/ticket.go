package ticket

import "time"

type Ticket struct {
	ID          int64     `db:"id"`
	Title       string    `db:"title"`
	Description string    `db:"description"`
	Status      string    `db:"status"`
	Priority    string    `db:"priority"`
	ProjectID   int64     `db:"project_id"`
	ReporterID  int64     `db:"reporter_id"`
	AssigneeID  *int64    `db:"assignee_id"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}
