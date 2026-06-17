package githubintegration

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/google/uuid"
)

// ErrInvalidInput reports malformed GitHub integration input.
var ErrInvalidInput = errors.New("invalid github integration input")

// ErrNotFound reports a missing local GitHub integration resource.
var ErrNotFound = errors.New("github integration resource not found")

var (
	// githubNamePattern accepts GitHub owner and repository path segments.
	githubNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	// fullUUIDPattern finds local ticket UUIDs in commit messages.
	fullUUIDPattern = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	// compactRefPattern finds compact local ticket references in commit messages.
	compactRefPattern = regexp.MustCompile(`(?i)(?:#|ticket:|task:|progo-)([0-9a-f]{8})\b`)
)

// ProjectPermissionChecker checks local project permissions.
type ProjectPermissionChecker interface {
	RequireProjectPermission(ctx context.Context, projectID, userID, permission, deniedMessage string) error
}

// Service manages project GitHub repository links and imported commits.
type Service struct {
	repo     Repository
	projects ProjectPermissionChecker
	client   Client
}

// NewService creates a GitHub integration service.
func NewService(repo Repository, projects ProjectPermissionChecker, client Client) *Service {
	return &Service{
		repo:     repo,
		projects: projects,
		client:   client,
	}
}

// LinkRepository attaches a GitHub repository to a project.
func (s *Service) LinkRepository(ctx context.Context, projectID, userID, owner, name string) (*GitHubRepository, error) {
	if err := validateUUID(projectID); err != nil {
		return nil, err
	}
	if err := s.requireProjectUpdate(ctx, projectID, userID); err != nil {
		return nil, err
	}
	owner, name, err := normalizeRepositoryInput(owner, name)
	if err != nil {
		return nil, err
	}
	remote, err := s.client.Repository(ctx, owner, name)
	if err != nil {
		if errors.Is(err, ErrGitHubNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if remote.Owner == "" {
		remote.Owner = owner
	}
	if remote.Name == "" {
		remote.Name = name
	}
	if remote.FullName == "" {
		remote.FullName = remote.Owner + "/" + remote.Name
	}

	repo := &GitHubRepository{
		ProjectID:     projectID,
		Owner:         strings.ToLower(remote.Owner),
		Name:          strings.ToLower(remote.Name),
		FullName:      remote.FullName,
		HTMLURL:       remote.HTMLURL,
		DefaultBranch: remote.DefaultBranch,
		CreatedBy:     &userID,
	}
	if err := s.repo.UpsertRepository(ctx, repo); err != nil {
		return nil, err
	}
	return repo, nil
}

// ListRepositories returns project GitHub repositories visible to a user.
func (s *Service) ListRepositories(ctx context.Context, projectID, userID string) ([]GitHubRepository, error) {
	if err := validateUUID(projectID); err != nil {
		return nil, err
	}
	if err := s.requireProjectRead(ctx, projectID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListRepositories(ctx, projectID)
}

// DeleteRepository removes a GitHub repository link from a project.
func (s *Service) DeleteRepository(ctx context.Context, projectID, userID, repositoryID string) error {
	if err := validateUUID(projectID); err != nil {
		return err
	}
	if err := validateUUID(repositoryID); err != nil {
		return err
	}
	if err := s.requireProjectUpdate(ctx, projectID, userID); err != nil {
		return err
	}
	if err := s.repo.DeleteRepository(ctx, projectID, repositoryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// SyncRepository imports recent commits and auto-links ticket references.
func (s *Service) SyncRepository(ctx context.Context, projectID, userID, repositoryID string) (*SyncResult, error) {
	if err := validateUUID(projectID); err != nil {
		return nil, err
	}
	if err := validateUUID(repositoryID); err != nil {
		return nil, err
	}
	if err := s.requireProjectUpdate(ctx, projectID, userID); err != nil {
		return nil, err
	}
	repo, err := s.repo.GetRepository(ctx, projectID, repositoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var since *time.Time
	if repo.LastSyncedAt != nil {
		value := repo.LastSyncedAt.Add(-1 * time.Minute)
		since = &value
	}
	remoteCommits, err := s.client.ListCommits(ctx, repo.Owner, repo.Name, repo.DefaultBranch, since, 100)
	if err != nil {
		return nil, err
	}

	prepared := make([]GitHubCommit, 0, len(remoteCommits))
	allRefs := make([]string, 0)
	for _, remote := range remoteCommits {
		sha := strings.TrimSpace(remote.SHA)
		if sha == "" {
			continue
		}
		commit := GitHubCommit{
			RepositoryID: repo.ID,
			SHA:          sha,
			ShortSHA:     shortSHA(sha),
			Message:      strings.TrimSpace(remote.Message),
			AuthorName:   strings.TrimSpace(remote.AuthorName),
			AuthorEmail:  strings.TrimSpace(remote.AuthorEmail),
			AuthoredAt:   remote.AuthoredAt,
			HTMLURL:      strings.TrimSpace(remote.HTMLURL),
		}
		prepared = append(prepared, commit)
		allRefs = append(allRefs, ticketReferences(commit.Message)...)
	}

	resolved, err := s.repo.FindTicketIDsByReferences(ctx, projectID, allRefs)
	if err != nil {
		return nil, err
	}
	imported := 0
	linked := 0
	for i := range prepared {
		refs := ticketReferences(prepared[i].Message)
		ticketIDs := make([]string, 0, len(refs))
		for _, ref := range refs {
			if ticketID := resolved[ref]; ticketID != "" {
				ticketIDs = append(ticketIDs, ticketID)
			}
		}
		newLinks, err := s.repo.UpsertCommitWithLinks(ctx, &prepared[i], ticketIDs, userID)
		if err != nil {
			return nil, err
		}
		imported++
		linked += newLinks
	}

	now := time.Now().UTC()
	if err := s.repo.MarkRepositorySynced(ctx, repo.ID, now); err != nil {
		return nil, err
	}
	repo.LastSyncedAt = &now
	return &SyncResult{Repository: *repo, Imported: imported, Linked: linked}, nil
}

// ListTicketCommits returns imported GitHub commits linked to a ticket.
func (s *Service) ListTicketCommits(ctx context.Context, ticketID, userID string) ([]GitHubCommit, error) {
	if err := validateUUID(ticketID); err != nil {
		return nil, err
	}
	projectID, err := s.repo.TicketProjectID(ctx, ticketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := s.requireProjectRead(ctx, projectID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListCommitsByTicket(ctx, projectID, ticketID)
}

// requireProjectRead checks read access to project GitHub metadata.
func (s *Service) requireProjectRead(ctx context.Context, projectID, userID string) error {
	return s.projects.RequireProjectPermission(ctx, projectID, userID, project.PermissionProjectRead, "project not found or access denied")
}

// requireProjectUpdate checks permission to manage project GitHub metadata.
func (s *Service) requireProjectUpdate(ctx context.Context, projectID, userID string) error {
	return s.projects.RequireProjectPermission(ctx, projectID, userID, project.PermissionProjectUpdate, "insufficient permissions: missing project.update")
}

// validateUUID checks that a route identifier is a UUID.
func validateUUID(value string) error {
	if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
		return ErrInvalidInput
	}
	return nil
}

// normalizeRepositoryInput validates GitHub owner and repository names.
func normalizeRepositoryInput(owner, name string) (string, string, error) {
	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	if owner == "" || name == "" {
		return "", "", ErrInvalidInput
	}
	if !githubNamePattern.MatchString(owner) || !githubNamePattern.MatchString(name) {
		return "", "", ErrInvalidInput
	}
	return owner, name, nil
}

// ticketReferences extracts supported local ticket references from commit messages.
func ticketReferences(message string) []string {
	refs := make([]string, 0)
	for _, match := range fullUUIDPattern.FindAllString(message, -1) {
		refs = append(refs, strings.ToLower(match))
	}
	for _, match := range compactRefPattern.FindAllStringSubmatch(message, -1) {
		if len(match) > 1 {
			refs = append(refs, strings.ToLower(match[1]))
		}
	}
	return uniqueStrings(refs)
}

// shortSHA returns a readable commit SHA prefix.
func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}
