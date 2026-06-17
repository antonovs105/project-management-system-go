package githubintegration

import "time"

// GitHubRepository is a repository attached to a local project.
type GitHubRepository struct {
	ID                string     `db:"id" json:"id"`
	ProjectID         string     `db:"project_id" json:"project_id"`
	Owner             string     `db:"owner" json:"owner"`
	Name              string     `db:"name" json:"name"`
	FullName          string     `db:"full_name" json:"full_name"`
	HTMLURL           string     `db:"html_url" json:"html_url"`
	DefaultBranch     string     `db:"default_branch" json:"default_branch"`
	LastSyncedAt      *time.Time `db:"last_synced_at" json:"last_synced_at"`
	LastSyncError     string     `db:"last_sync_error" json:"last_sync_error"`
	LastWebhookAt     *time.Time `db:"last_webhook_at" json:"last_webhook_at"`
	CommitCount       int        `db:"commit_count" json:"commit_count"`
	LinkedCommitCount int        `db:"linked_commit_count" json:"linked_commit_count"`
	ManualLinkCount   int        `db:"manual_link_count" json:"manual_link_count"`
	CreatedBy         *string    `db:"created_by" json:"created_by"`
	CreatedAt         time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at" json:"updated_at"`
}

// GitHubCommit is a normalized commit imported from an attached repository.
type GitHubCommit struct {
	ID                 string     `db:"id" json:"id"`
	RepositoryID       string     `db:"repository_id" json:"repository_id"`
	RepositoryFullName string     `db:"repository_full_name" json:"repository_full_name"`
	RepositoryHTMLURL  string     `db:"repository_html_url" json:"repository_html_url"`
	SHA                string     `db:"sha" json:"sha"`
	ShortSHA           string     `db:"short_sha" json:"short_sha"`
	Message            string     `db:"message" json:"message"`
	AuthorName         string     `db:"author_name" json:"author_name"`
	AuthorEmail        string     `db:"author_email" json:"author_email"`
	AuthoredAt         *time.Time `db:"authored_at" json:"authored_at"`
	HTMLURL            string     `db:"html_url" json:"html_url"`
	TicketIDs          []string   `db:"-" json:"ticket_ids"`
	LinkSource         string     `db:"link_source" json:"link_source"`
	CreatedAt          time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt          time.Time  `db:"updated_at" json:"updated_at"`
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

// CommitListOptions filters imported commits for a project.
type CommitListOptions struct {
	RepositoryID string
	Query        string
	UnlinkedOnly bool
	Limit        int
}

// WebhookResult summarizes one GitHub webhook delivery.
type WebhookResult struct {
	Event        string `json:"event"`
	DeliveryID   string `json:"delivery_id"`
	Repositories int    `json:"repositories"`
	Imported     int    `json:"imported"`
	Linked       int    `json:"linked"`
	Ignored      bool   `json:"ignored"`
}
