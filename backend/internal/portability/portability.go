// Package portability implements versioned project and account data transfer.
package portability

import "time"

const (
	// ProjectSchema identifies the stable project bundle format.
	ProjectSchema = "progo.project.v1"
	// UserSchema identifies the stable account bundle format.
	UserSchema = "progo.user.v1"
)

// ProjectBundle is a self-describing, implementation-neutral project export.
type ProjectBundle struct {
	Schema     string         `json:"schema"`
	ExportedAt time.Time      `json:"exported_at"`
	Project    ExportProject  `json:"project"`
	Members    []ExportMember `json:"members"`
	Labels     []ExportLabel  `json:"labels"`
	Tickets    []ExportTicket `json:"tickets"`
}

// ExportProject contains portable project metadata.
type ExportProject struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ExportMember preserves project membership context without authentication data.
type ExportMember struct {
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	Handle   string `json:"handle"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Remote   bool   `json:"remote"`
}

// ExportLabel is a project-scoped portable label.
type ExportLabel struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// ExportTicket contains a ticket, hierarchy references, labels, and comments.
type ExportTicket struct {
	SourceID       string          `json:"source_id"`
	ParentSourceID *string         `json:"parent_source_id,omitempty"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	Status         string          `json:"status"`
	Priority       string          `json:"priority"`
	Type           string          `json:"type"`
	DueDate        *time.Time      `json:"due_date,omitempty"`
	Labels         []string        `json:"labels"`
	Comments       []ExportComment `json:"comments"`
}

// ExportComment preserves comment content and source attribution metadata.
type ExportComment struct {
	AuthorSourceID string    `json:"author_source_id"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
}

// ImportResult summarizes a completed project or bulk ticket import.
type ImportResult struct {
	ProjectID        string `json:"project_id"`
	LabelsImported   int    `json:"labels_imported"`
	TicketsImported  int    `json:"tickets_imported"`
	CommentsImported int    `json:"comments_imported"`
}

// UserBundle exports one local identity and every currently accessible project.
type UserBundle struct {
	Schema     string          `json:"schema"`
	ExportedAt time.Time       `json:"exported_at"`
	Account    ExportAccount   `json:"account"`
	Projects   []ProjectBundle `json:"projects"`
}

// ExportAccount contains portable local profile data and no credentials.
type ExportAccount struct {
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	Handle        string    `json:"handle"`
	Name          string    `json:"name"`
	Summary       string    `json:"summary"`
	CreatedAt     time.Time `json:"created_at"`
}
