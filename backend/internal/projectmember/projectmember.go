package projectmember

import "time"

type ProjectMember struct {
	UserID    string    `db:"user_id" json:"user_id"`
	ProjectID string    `db:"project_id" json:"project_id"`
	Role      string    `db:"role" json:"role"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
