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
	ListRepositoriesByRemote(ctx context.Context, owner, name string) ([]GitHubRepository, error)
	GetRepository(ctx context.Context, projectID, repositoryID string) (*GitHubRepository, error)
	DeleteRepository(ctx context.Context, projectID, repositoryID string) error
	MarkRepositorySynced(ctx context.Context, repositoryID string, syncedAt time.Time) error
	MarkRepositorySyncFailed(ctx context.Context, repositoryID, message string) error
	MarkRepositoryWebhookReceived(ctx context.Context, repositoryID string, receivedAt time.Time) error
	UpsertCommitWithLinks(ctx context.Context, commit *GitHubCommit, ticketIDs []string, actorID string) (int, error)
	ListCommitsByProject(ctx context.Context, projectID string, options CommitListOptions) ([]GitHubCommit, error)
	ListCommitsByTicket(ctx context.Context, projectID, ticketID string) ([]GitHubCommit, error)
	LinkCommitToTicket(ctx context.Context, projectID, ticketID, commitID, actorID string) (*GitHubCommit, error)
	UnlinkCommitFromTicket(ctx context.Context, projectID, ticketID, commitID string) error
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
			last_sync_error,
			last_webhook_at,
			0 AS commit_count,
			0 AS linked_commit_count,
			0 AS manual_link_count,
			created_by::text,
			created_at,
			updated_at
	`, repo.ProjectID, repo.Owner, repo.Name, repo.FullName, repo.HTMLURL, repo.DefaultBranch, repo.CreatedBy)
}

// ListRepositories returns repositories attached to a project.
func (r *PgRepository) ListRepositories(ctx context.Context, projectID string) ([]GitHubRepository, error) {
	repos := make([]GitHubRepository, 0)
	if err := r.db.SelectContext(ctx, &repos, repositorySelectQuery()+`
		WHERE r.project_id = $1
		ORDER BY owner, name
	`, projectID); err != nil {
		return nil, err
	}
	return repos, nil
}

// ListRepositoriesByRemote returns all local project links for one GitHub repository.
func (r *PgRepository) ListRepositoriesByRemote(ctx context.Context, owner, name string) ([]GitHubRepository, error) {
	repos := make([]GitHubRepository, 0)
	if err := r.db.SelectContext(ctx, &repos, repositorySelectQuery()+`
		WHERE r.owner = $1 AND r.name = $2
		ORDER BY r.project_id
	`, strings.ToLower(owner), strings.ToLower(name)); err != nil {
		return nil, err
	}
	return repos, nil
}

// GetRepository returns one repository attached to a project.
func (r *PgRepository) GetRepository(ctx context.Context, projectID, repositoryID string) (*GitHubRepository, error) {
	var repo GitHubRepository
	if err := r.db.GetContext(ctx, &repo, repositorySelectQuery()+`
		WHERE r.project_id = $1 AND r.id = $2
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
		SET last_synced_at = $2, last_sync_error = '', updated_at = now()
		WHERE id = $1
	`, repositoryID, syncedAt)
	return err
}

// MarkRepositorySyncFailed records the most recent manual sync failure.
func (r *PgRepository) MarkRepositorySyncFailed(ctx context.Context, repositoryID, message string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE project_github_repositories
		SET last_sync_error = $2, updated_at = now()
		WHERE id = $1
	`, repositoryID, truncateStatusMessage(message))
	return err
}

// MarkRepositoryWebhookReceived records a verified webhook import timestamp.
func (r *PgRepository) MarkRepositoryWebhookReceived(ctx context.Context, repositoryID string, receivedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE project_github_repositories
		SET last_webhook_at = $2, last_synced_at = $2, last_sync_error = '', updated_at = now()
		WHERE id = $1
	`, repositoryID, receivedAt)
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

// ListCommitsByProject returns imported commits for a project.
func (r *PgRepository) ListCommitsByProject(ctx context.Context, projectID string, options CommitListOptions) ([]GitHubCommit, error) {
	rows := make([]githubCommitRow, 0)
	if err := r.db.SelectContext(ctx, &rows, `
		SELECT
			c.id::text,
			c.repository_id::text,
			r.full_name AS repository_full_name,
			r.html_url AS repository_html_url,
			c.sha,
			c.short_sha,
			c.message,
			c.author_name,
			c.author_email,
			c.authored_at,
			c.html_url,
			COALESCE(array_remove(array_agg(l.ticket_id::text ORDER BY l.created_at), NULL), ARRAY[]::text[]) AS ticket_ids,
			'' AS link_source,
			c.created_at,
			c.updated_at
		FROM github_commits c
		JOIN project_github_repositories r ON r.id = c.repository_id
		LEFT JOIN github_commit_ticket_links l ON l.commit_id = c.id
		WHERE r.project_id = $1
			AND (NULLIF($2, '')::uuid IS NULL OR c.repository_id = NULLIF($2, '')::uuid)
			AND (
				NULLIF($3, '') IS NULL
				OR c.sha ILIKE '%' || $3 || '%'
				OR c.short_sha ILIKE '%' || $3 || '%'
				OR c.message ILIKE '%' || $3 || '%'
				OR c.author_name ILIKE '%' || $3 || '%'
				OR r.full_name ILIKE '%' || $3 || '%'
			)
			AND (
				$4 = false
				OR NOT EXISTS (
					SELECT 1
					FROM github_commit_ticket_links existing
					WHERE existing.commit_id = c.id
				)
			)
		GROUP BY c.id, r.full_name, r.html_url
		ORDER BY c.authored_at DESC NULLS LAST, c.created_at DESC
		LIMIT $5
	`, projectID, options.RepositoryID, strings.TrimSpace(options.Query), options.UnlinkedOnly, normalizeCommitListLimit(options.Limit)); err != nil {
		return nil, err
	}
	return commitRows(rows), nil
}

// ListCommitsByTicket returns imported commits linked to a local ticket.
func (r *PgRepository) ListCommitsByTicket(ctx context.Context, projectID, ticketID string) ([]GitHubCommit, error) {
	rows := make([]githubCommitRow, 0)
	if err := r.db.SelectContext(ctx, &rows, `
		SELECT
			c.id::text,
			c.repository_id::text,
			r.full_name AS repository_full_name,
			r.html_url AS repository_html_url,
			c.sha,
			c.short_sha,
			c.message,
			c.author_name,
			c.author_email,
			c.authored_at,
			c.html_url,
			ARRAY[l.ticket_id::text]::text[] AS ticket_ids,
			l.link_source,
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
	return commitRows(rows), nil
}

// LinkCommitToTicket attaches one imported commit to a ticket in the same project.
func (r *PgRepository) LinkCommitToTicket(ctx context.Context, projectID, ticketID, commitID, actorID string) (*GitHubCommit, error) {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO github_commit_ticket_links (commit_id, ticket_id, link_source, created_by)
		SELECT c.id, t.id, 'manual', NULLIF($4, '')::uuid
		FROM github_commits c
		JOIN project_github_repositories r ON r.id = c.repository_id
		JOIN tickets t ON t.project_id = r.project_id AND t.id = $2
		WHERE r.project_id = $1 AND c.id = $3
		ON CONFLICT (commit_id, ticket_id) DO NOTHING
	`, projectID, ticketID, commitID, actorID)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		if _, err := r.getCommitByID(ctx, projectID, commitID); err != nil {
			return nil, err
		}
		if projectIDForTicket, err := r.TicketProjectID(ctx, ticketID); err != nil || projectIDForTicket != projectID {
			return nil, sql.ErrNoRows
		}
	}
	return r.getCommitByID(ctx, projectID, commitID)
}

// UnlinkCommitFromTicket removes one commit-ticket attachment.
func (r *PgRepository) UnlinkCommitFromTicket(ctx context.Context, projectID, ticketID, commitID string) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM github_commit_ticket_links l
		USING github_commits c, project_github_repositories r, tickets t
		WHERE l.commit_id = c.id
			AND c.repository_id = r.id
			AND l.ticket_id = t.id
			AND r.project_id = $1
			AND t.project_id = $1
			AND l.ticket_id = $2
			AND l.commit_id = $3
	`, projectID, ticketID, commitID)
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
	shortRefSet := makeStringSet(shortRefs)
	rows, err := r.db.QueryxContext(ctx, `
		WITH candidates AS (
			SELECT
				id::text AS ticket_id,
				lower(id::text) AS full_ref,
				left(lower(id::text), 8) AS short_ref,
				count(*) OVER (PARTITION BY left(lower(id::text), 8)) AS short_ref_count
			FROM tickets
			WHERE project_id = $1
				AND (
					lower(id::text) = ANY($2)
					OR left(lower(id::text), 8) = ANY($3)
				)
		)
		SELECT
			ticket_id,
			full_ref,
			CASE WHEN short_ref_count = 1 THEN short_ref ELSE '' END AS short_ref
		FROM candidates
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
		if _, requested := shortRefSet[shortRef]; requested {
			resolved[shortRef] = ticketID
		}
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
			r.id::text,
			r.project_id::text,
			r.owner,
			r.name,
			r.full_name,
			r.html_url,
			r.default_branch,
			r.last_synced_at,
			r.last_sync_error,
			r.last_webhook_at,
			(
				SELECT count(*)
				FROM github_commits c
				WHERE c.repository_id = r.id
			) AS commit_count,
			(
				SELECT count(DISTINCT c.id)
				FROM github_commits c
				JOIN github_commit_ticket_links l ON l.commit_id = c.id
				WHERE c.repository_id = r.id
			) AS linked_commit_count,
			(
				SELECT count(*)
				FROM github_commits c
				JOIN github_commit_ticket_links l ON l.commit_id = c.id
				WHERE c.repository_id = r.id AND l.link_source = 'manual'
			) AS manual_link_count,
			r.created_by::text,
			r.created_at,
			r.updated_at
		FROM project_github_repositories r
	`
}

// getCommitByID returns one imported commit and its current ticket links.
func (r *PgRepository) getCommitByID(ctx context.Context, projectID, commitID string) (*GitHubCommit, error) {
	rows := make([]githubCommitRow, 0, 1)
	if err := r.db.SelectContext(ctx, &rows, `
		SELECT
			c.id::text,
			c.repository_id::text,
			r.full_name AS repository_full_name,
			r.html_url AS repository_html_url,
			c.sha,
			c.short_sha,
			c.message,
			c.author_name,
			c.author_email,
			c.authored_at,
			c.html_url,
			COALESCE(array_remove(array_agg(l.ticket_id::text ORDER BY l.created_at), NULL), ARRAY[]::text[]) AS ticket_ids,
			'' AS link_source,
			c.created_at,
			c.updated_at
		FROM github_commits c
		JOIN project_github_repositories r ON r.id = c.repository_id
		LEFT JOIN github_commit_ticket_links l ON l.commit_id = c.id
		WHERE r.project_id = $1 AND c.id = $2
		GROUP BY c.id, r.full_name, r.html_url
	`, projectID, commitID); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	commits := commitRows(rows)
	return &commits[0], nil
}

// githubCommitRow scans commit rows with PostgreSQL array support.
type githubCommitRow struct {
	ID                 string         `db:"id"`
	RepositoryID       string         `db:"repository_id"`
	RepositoryFullName string         `db:"repository_full_name"`
	RepositoryHTMLURL  string         `db:"repository_html_url"`
	SHA                string         `db:"sha"`
	ShortSHA           string         `db:"short_sha"`
	Message            string         `db:"message"`
	AuthorName         string         `db:"author_name"`
	AuthorEmail        string         `db:"author_email"`
	AuthoredAt         *time.Time     `db:"authored_at"`
	HTMLURL            string         `db:"html_url"`
	TicketIDs          pq.StringArray `db:"ticket_ids"`
	LinkSource         string         `db:"link_source"`
	CreatedAt          time.Time      `db:"created_at"`
	UpdatedAt          time.Time      `db:"updated_at"`
}

// commit converts a scanned row into the API commit DTO.
func (row githubCommitRow) commit() GitHubCommit {
	ticketIDs := make([]string, len(row.TicketIDs))
	copy(ticketIDs, row.TicketIDs)
	return GitHubCommit{
		ID:                 row.ID,
		RepositoryID:       row.RepositoryID,
		RepositoryFullName: row.RepositoryFullName,
		RepositoryHTMLURL:  row.RepositoryHTMLURL,
		SHA:                row.SHA,
		ShortSHA:           row.ShortSHA,
		Message:            row.Message,
		AuthorName:         row.AuthorName,
		AuthorEmail:        row.AuthorEmail,
		AuthoredAt:         row.AuthoredAt,
		HTMLURL:            row.HTMLURL,
		TicketIDs:          ticketIDs,
		LinkSource:         row.LinkSource,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

// commitRows converts scanned rows into commit DTOs.
func commitRows(rows []githubCommitRow) []GitHubCommit {
	commits := make([]GitHubCommit, 0, len(rows))
	for _, row := range rows {
		commits = append(commits, row.commit())
	}
	return commits
}

// normalizeCommitListLimit bounds project commit list size.
func normalizeCommitListLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// truncateStatusMessage keeps persisted status messages reasonably small.
func truncateStatusMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 500 {
		return message[:500]
	}
	return message
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

// makeStringSet returns trimmed non-empty values as a lookup set.
func makeStringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}
