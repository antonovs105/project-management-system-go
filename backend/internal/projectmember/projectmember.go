package projectmember

import "time"

type ProjectMember struct {
	UserID    int64     `db:"user_id"`
	ProjectID int64     `db:"project_id"`
	Role      string    `db:"role"`
	CreatedAt time.Time `db:"created_at"`
}
