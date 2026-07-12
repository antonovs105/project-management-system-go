// Package label implements project-scoped ticket labels.
package label

import "time"

// Label is a reusable project-scoped ticket tag.
type Label struct {
	ID        string    `db:"id" json:"id"`
	ProjectID string    `db:"project_id" json:"project_id"`
	Name      string    `db:"name" json:"name"`
	Color     string    `db:"color" json:"color"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
