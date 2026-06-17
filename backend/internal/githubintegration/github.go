package githubintegration

import "time"

// GitHubRepository is a repository attached to a local project.
type GitHubRepository struct {
	ID            string     `db:"id" json:"id"`
	ProjectID     string     `db:"project_id" json:"project_id"`
	Owner         string     `db:"owner" json:"owner"`
	Name          string     `db:"name" json:"name"`
	FullName      string     `db:"full_name" json:"full_name"`
	HTMLURL       string     `db:"html_url" json:"html_url"`
	DefaultBranch string     `db:"default_branch" json:"default_branch"`
	LastSyncedAt  *time.Time `db:"last_synced_at" json:"last_synced_at"`
	CreatedBy     *string    `db:"created_by" json:"created_by"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at" json:"updated_at"`
}

// GitHubCommit is a normalized commit imported from an attached repository.
type GitHubCommit struct {
	ID           string     `db:"id" json:"id"`
	RepositoryID string     `db:"repository_id" json:"repository_id"`
	SHA          string     `db:"sha" json:"sha"`
	ShortSHA     string     `db:"short_sha" json:"short_sha"`
	Message      string     `db:"message" json:"message"`
	AuthorName   string     `db:"author_name" json:"author_name"`
	AuthorEmail  string     `db:"author_email" json:"author_email"`
	AuthoredAt   *time.Time `db:"authored_at" json:"authored_at"`
	HTMLURL      string     `db:"html_url" json:"html_url"`
	TicketIDs    []string   `db:"-" json:"ticket_ids"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
}

// RemoteRepository is repository metadata returned by GitHub.
type RemoteRepository struct {
	Owner         string
	Name          string
	FullName      string
	HTMLURL       string
	DefaultBranch string
}

// RemoteCommit is commit metadata returned by GitHub.
type RemoteCommit struct {
	SHA         string
	Message     string
	AuthorName  string
	AuthorEmail string
	AuthoredAt  *time.Time
	HTMLURL     string
}

// SyncResult summarizes an import run for one repository.
type SyncResult struct {
	Repository GitHubRepository `json:"repository"`
	Imported   int              `json:"imported"`
	Linked     int              `json:"linked"`
}
