package githubintegration

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// Repository defines persistence for project GitHub links and imported commits.
type Repository interface {
	UpsertRepository(ctx context.Context, repo *GitHubRepository) error
	ListRepositories(ctx context.Context, projectID string) ([]GitHubRepository, error)
	GetRepository(ctx context.Context, projectID, repositoryID string) (*GitHubRepository, error)
	DeleteRepository(ctx context.Context, projectID, repositoryID string) error
	MarkRepositorySynced(ctx context.Context, repositoryID string, syncedAt time.Time) error
	UpsertCommitWithLinks(ctx context.Context, commit *GitHubCommit, ticketIDs []string, actorID string) (int, error)
	ListCommitsByTicket(ctx context.Context, projectID, ticketID string) ([]GitHubCommit, error)
	FindTicketIDsByReferences(ctx context.Context, projectID string, refs []string) (map[string]string, error)
	TicketProjectID(ctx context.Context, ticketID string) (string, error)
}

// PgRepository implements Repository using PostgreSQL.
type PgRepository struct {
	db *sqlx.DB
}

// NewRepository creates a PostgreSQL-backed GitHub integration repository.
func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

// UpsertRepository creates or refreshes a project repository link.
func (r *PgRepository) UpsertRepository(ctx context.Context, repo *GitHubRepository) error {
	return r.db.GetContext(ctx, repo, `
		INSERT INTO project_github_repositories (
			project_id, owner, name, full_name, html_url, default_branch, created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (project_id, owner, name)
		DO UPDATE SET
			full_name = EXCLUDED.full_name,
			html_url = EXCLUDED.html_url,
			default_branch = EXCLUDED.default_branch,
			updated_at = now()
		RETURNING
			id::text,
			project_id::text,
			owner,
			name,
			full_name,
			html_url,
			default_branch,
			last_synced_at,
			created_by::text,
			created_at,
			updated_at
	`, repo.ProjectID, repo.Owner, repo.Name, repo.FullName, repo.HTMLURL, repo.DefaultBranch, repo.CreatedBy)
}

// ListRepositories returns repositories attached to a project.
func (r *PgRepository) ListRepositories(ctx context.Context, projectID string) ([]GitHubRepository, error) {
	repos := make([]GitHubRepository, 0)
	if err := r.db.SelectContext(ctx, &repos, repositorySelectQuery()+`
		WHERE project_id = $1
		ORDER BY owner, name
	`, projectID); err != nil {
		return nil, err
	}
	return repos, nil
}

// GetRepository returns one repository attached to a project.
func (r *PgRepository) GetRepository(ctx context.Context, projectID, repositoryID string) (*GitHubRepository, error) {
	var repo GitHubRepository
	if err := r.db.GetContext(ctx, &repo, repositorySelectQuery()+`
		WHERE project_id = $1 AND id = $2
	`, projectID, repositoryID); err != nil {
		return nil, err
	}
	return &repo, nil
}

// DeleteRepository removes one project repository link and its imported commits.
func (r *PgRepository) DeleteRepository(ctx context.Context, projectID, repositoryID string) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM project_github_repositories
		WHERE project_id = $1 AND id = $2
	`, projectID, repositoryID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// MarkRepositorySynced records a completed commit sync timestamp.
func (r *PgRepository) MarkRepositorySynced(ctx context.Context, repositoryID string, syncedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE project_github_repositories
		SET last_synced_at = $2, updated_at = now()
		WHERE id = $1
	`, repositoryID, syncedAt)
	return err
}

// UpsertCommitWithLinks imports a commit and stores ticket links found in its message.
func (r *PgRepository) UpsertCommitWithLinks(ctx context.Context, commit *GitHubCommit, ticketIDs []string, actorID string) (int, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if err := tx.GetContext(ctx, commit, `
		INSERT INTO github_commits (
			repository_id, sha, short_sha, message, author_name, author_email, authored_at, html_url
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (repository_id, sha)
		DO UPDATE SET
			short_sha = EXCLUDED.short_sha,
			message = EXCLUDED.message,
			author_name = EXCLUDED.author_name,
			author_email = EXCLUDED.author_email,
			authored_at = EXCLUDED.authored_at,
			html_url = EXCLUDED.html_url,
			updated_at = now()
		RETURNING
			id::text,
			repository_id::text,
			sha,
			short_sha,
			message,
			author_name,
			author_email,
			authored_at,
			html_url,
			created_at,
			updated_at
	`, commit.RepositoryID, commit.SHA, commit.ShortSHA, commit.Message, commit.AuthorName, commit.AuthorEmail, commit.AuthoredAt, commit.HTMLURL); err != nil {
		return 0, err
	}

	linked := 0
	for _, ticketID := range uniqueStrings(ticketIDs) {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO github_commit_ticket_links (commit_id, ticket_id, link_source, created_by)
			VALUES ($1, $2, 'message', NULLIF($3, '')::uuid)
			ON CONFLICT (commit_id, ticket_id) DO NOTHING
		`, commit.ID, ticketID, actorID)
		if err != nil {
			return 0, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		linked += int(affected)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return linked, nil
}

// ListCommitsByTicket returns imported commits linked to a local ticket.
func (r *PgRepository) ListCommitsByTicket(ctx context.Context, projectID, ticketID string) ([]GitHubCommit, error) {
	commits := make([]GitHubCommit, 0)
	if err := r.db.SelectContext(ctx, &commits, `
		SELECT
			c.id::text,
			c.repository_id::text,
			c.sha,
			c.short_sha,
			c.message,
			c.author_name,
			c.author_email,
			c.authored_at,
			c.html_url,
			c.created_at,
			c.updated_at
		FROM github_commits c
		JOIN github_commit_ticket_links l ON l.commit_id = c.id
		JOIN project_github_repositories r ON r.id = c.repository_id
		WHERE r.project_id = $1 AND l.ticket_id = $2
		ORDER BY c.authored_at DESC NULLS LAST, c.created_at DESC
	`, projectID, ticketID); err != nil {
		return nil, err
	}
	for i := range commits {
		commits[i].TicketIDs = []string{ticketID}
	}
	return commits, nil
}

// FindTicketIDsByReferences resolves full UUIDs and compact prefixes inside one project.
func (r *PgRepository) FindTicketIDsByReferences(ctx context.Context, projectID string, refs []string) (map[string]string, error) {
	fullRefs := make([]string, 0, len(refs))
	shortRefs := make([]string, 0, len(refs))
	for _, ref := range refs {
		normalized := strings.ToLower(strings.TrimSpace(ref))
		if normalized == "" {
			continue
		}
		if len(normalized) == 36 {
			fullRefs = append(fullRefs, normalized)
			continue
		}
		shortRefs = append(shortRefs, normalized)
	}
	rows, err := r.db.QueryxContext(ctx, `
		SELECT id::text, lower(id::text), left(lower(id::text), 8)
		FROM tickets
		WHERE project_id = $1
			AND (
				lower(id::text) = ANY($2)
				OR left(lower(id::text), 8) = ANY($3)
			)
	`, projectID, pq.Array(fullRefs), pq.Array(shortRefs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	resolved := make(map[string]string)
	for rows.Next() {
		var ticketID string
		var fullRef string
		var shortRef string
		if err := rows.Scan(&ticketID, &fullRef, &shortRef); err != nil {
			return nil, err
		}
		resolved[fullRef] = ticketID
		resolved[shortRef] = ticketID
	}
	return resolved, rows.Err()
}

// TicketProjectID returns the project that owns a ticket.
func (r *PgRepository) TicketProjectID(ctx context.Context, ticketID string) (string, error) {
	var projectID string
	err := r.db.GetContext(ctx, &projectID, `SELECT project_id::text FROM tickets WHERE id = $1`, ticketID)
	return projectID, err
}

// repositorySelectQuery returns the repository projection shared by list and get queries.
func repositorySelectQuery() string {
	return `
		SELECT
			id::text,
			project_id::text,
			owner,
			name,
			full_name,
			html_url,
			default_branch,
			last_synced_at,
			created_by::text,
			created_at,
			updated_at
		FROM project_github_repositories
	`
}

// uniqueStrings removes empty duplicates while preserving first-seen order.
func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
