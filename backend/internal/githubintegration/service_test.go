package githubintegration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProjectChecker struct {
	err        error
	permission string
}

func (f *fakeProjectChecker) RequireProjectPermission(_ context.Context, _, _ string, permission string, _ string) error {
	f.permission = permission
	return f.err
}

type fakeClient struct {
	repository RemoteRepository
	commits    []RemoteCommit
	err        error
}

func (f fakeClient) Repository(context.Context, string, string) (RemoteRepository, error) {
	return f.repository, f.err
}

func (f fakeClient) ListCommits(context.Context, string, string, string, *time.Time, int) ([]RemoteCommit, error) {
	return f.commits, f.err
}

type fakeRepository struct {
	repository   GitHubRepository
	repositories []GitHubRepository
	upsertedRepo *GitHubRepository
	upserts      []GitHubCommit
	linkSets     [][]string
	resolvedRefs map[string]string
	syncedAt     *time.Time
	failedSync   string
	webhookAt    *time.Time
	linkedCommit *GitHubCommit
	unlinked     bool
}

func (f *fakeRepository) UpsertRepository(_ context.Context, repo *GitHubRepository) error {
	copy := *repo
	copy.ID = "11111111-1111-4111-8111-111111111111"
	f.upsertedRepo = &copy
	*repo = copy
	return nil
}

func (f *fakeRepository) ListRepositories(context.Context, string) ([]GitHubRepository, error) {
	return []GitHubRepository{f.repository}, nil
}

func (f *fakeRepository) ListRepositoriesByRemote(context.Context, string, string) ([]GitHubRepository, error) {
	if f.repositories != nil {
		return f.repositories, nil
	}
	return []GitHubRepository{f.repository}, nil
}

func (f *fakeRepository) GetRepository(context.Context, string, string) (*GitHubRepository, error) {
	return &f.repository, nil
}

func (f *fakeRepository) DeleteRepository(context.Context, string, string) error {
	return nil
}

func (f *fakeRepository) MarkRepositorySynced(_ context.Context, _ string, syncedAt time.Time) error {
	f.syncedAt = &syncedAt
	return nil
}

func (f *fakeRepository) MarkRepositorySyncFailed(_ context.Context, _, message string) error {
	f.failedSync = message
	return nil
}

func (f *fakeRepository) MarkRepositoryWebhookReceived(_ context.Context, _ string, receivedAt time.Time) error {
	f.webhookAt = &receivedAt
	return nil
}

func (f *fakeRepository) UpsertCommitWithLinks(_ context.Context, commit *GitHubCommit, ticketIDs []string, _ string) (int, error) {
	copy := *commit
	copy.ID = "commit-" + commit.ShortSHA
	f.upserts = append(f.upserts, copy)
	f.linkSets = append(f.linkSets, ticketIDs)
	return len(ticketIDs), nil
}

func (f *fakeRepository) ListCommitsByProject(context.Context, string, CommitListOptions) ([]GitHubCommit, error) {
	return nil, nil
}

func (f *fakeRepository) ListCommitsByTicket(context.Context, string, string) ([]GitHubCommit, error) {
	return nil, nil
}

func (f *fakeRepository) LinkCommitToTicket(context.Context, string, string, string, string) (*GitHubCommit, error) {
	if f.linkedCommit != nil {
		return f.linkedCommit, nil
	}
	return &GitHubCommit{ID: "44444444-4444-4444-8444-444444444444"}, nil
}

func (f *fakeRepository) UnlinkCommitFromTicket(context.Context, string, string, string) error {
	f.unlinked = true
	return nil
}

func (f *fakeRepository) FindTicketIDsByReferences(_ context.Context, _ string, refs []string) (map[string]string, error) {
	return f.resolvedRefs, nil
}

func (f *fakeRepository) TicketProjectID(context.Context, string) (string, error) {
	return "22222222-2222-4222-8222-222222222222", nil
}

func TestService_LinkRepository(t *testing.T) {
	repo := &fakeRepository{}
	projects := &fakeProjectChecker{}
	service := NewService(repo, projects, fakeClient{repository: RemoteRepository{
		Owner:         "Antonovs105",
		Name:          "Progo",
		FullName:      "Antonovs105/Progo",
		HTMLURL:       "https://github.com/Antonovs105/Progo",
		DefaultBranch: "main",
	}})

	linked, err := service.LinkRepository(
		context.Background(),
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
		"Antonovs105",
		"Progo",
	)

	require.NoError(t, err)
	require.NotNil(t, linked)
	assert.Equal(t, "project.update", projects.permission)
	assert.Equal(t, "antonovs105", linked.Owner)
	assert.Equal(t, "progo", linked.Name)
	assert.Equal(t, "Antonovs105/Progo", linked.FullName)
	assert.NotNil(t, repo.upsertedRepo.CreatedBy)
}

func TestService_LinkRepositoryRejectsInvalidInput(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakeProjectChecker{}, fakeClient{})

	linked, err := service.LinkRepository(
		context.Background(),
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
		"bad owner",
		"repo",
	)

	assert.ErrorIs(t, err, ErrInvalidInput)
	assert.Nil(t, linked)
}

func TestService_LinkRepositoryPropagatesPermissionError(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakeProjectChecker{err: errors.New("insufficient permissions")}, fakeClient{})

	linked, err := service.LinkRepository(
		context.Background(),
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
		"owner",
		"repo",
	)

	assert.EqualError(t, err, "insufficient permissions")
	assert.Nil(t, linked)
}

func TestService_SyncRepositoryImportsAndLinksCommits(t *testing.T) {
	ticketID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	repo := &fakeRepository{
		repository: GitHubRepository{
			ID:            "11111111-1111-4111-8111-111111111111",
			ProjectID:     "22222222-2222-4222-8222-222222222222",
			Owner:         "owner",
			Name:          "repo",
			DefaultBranch: "main",
		},
		resolvedRefs: map[string]string{
			"aaaaaaaa": ticketID,
		},
	}
	authoredAt := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	service := NewService(repo, &fakeProjectChecker{}, fakeClient{commits: []RemoteCommit{
		{
			SHA:        "abcdef1234567890",
			Message:    "Fix #aaaaaaaa",
			AuthorName: "Dev",
			AuthoredAt: &authoredAt,
			HTMLURL:    "https://github.com/owner/repo/commit/abcdef",
		},
		{
			SHA:     "1234567890abcdef",
			Message: "No ticket reference",
		},
	}})

	result, err := service.SyncRepository(
		context.Background(),
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
		"11111111-1111-4111-8111-111111111111",
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.Imported)
	assert.Equal(t, 1, result.Linked)
	require.Len(t, repo.upserts, 2)
	assert.Equal(t, "abcdef123456", repo.upserts[0].ShortSHA)
	assert.Equal(t, []string{ticketID}, repo.linkSets[0])
	assert.Empty(t, repo.linkSets[1])
	assert.NotNil(t, repo.syncedAt)
}

func TestService_LinkCommitToTicketRequiresTicketUpdate(t *testing.T) {
	projects := &fakeProjectChecker{}
	repo := &fakeRepository{
		linkedCommit: &GitHubCommit{ID: "44444444-4444-4444-8444-444444444444"},
	}
	service := NewService(repo, projects, fakeClient{})

	commit, err := service.LinkCommitToTicket(
		context.Background(),
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444",
	)

	require.NoError(t, err)
	require.NotNil(t, commit)
	assert.Equal(t, "tickets.update", projects.permission)
	assert.Equal(t, "44444444-4444-4444-8444-444444444444", commit.ID)
}

func TestService_UnlinkCommitFromTicketRequiresTicketUpdate(t *testing.T) {
	projects := &fakeProjectChecker{}
	repo := &fakeRepository{}
	service := NewService(repo, projects, fakeClient{})

	err := service.UnlinkCommitFromTicket(
		context.Background(),
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444",
	)

	require.NoError(t, err)
	assert.Equal(t, "tickets.update", projects.permission)
	assert.True(t, repo.unlinked)
}

func TestService_ProcessWebhookImportsPushCommits(t *testing.T) {
	ticketID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	repo := &fakeRepository{
		repositories: []GitHubRepository{
			{
				ID:        "11111111-1111-4111-8111-111111111111",
				ProjectID: "22222222-2222-4222-8222-222222222222",
				Owner:     "owner",
				Name:      "repo",
				FullName:  "owner/repo",
			},
		},
		resolvedRefs: map[string]string{
			"aaaaaaaa": ticketID,
		},
	}
	service := NewService(repo, &fakeProjectChecker{}, fakeClient{})
	body := []byte(`{
		"ref": "refs/heads/main",
		"repository": {
			"name": "repo",
			"full_name": "owner/repo",
			"html_url": "https://github.com/owner/repo",
			"default_branch": "main",
			"owner": {"login": "owner"}
		},
		"commits": [
			{
				"id": "abcdef1234567890",
				"message": "Fix #aaaaaaaa",
				"timestamp": "2026-06-17T12:00:00Z",
				"url": "https://github.com/owner/repo/commit/abcdef",
				"author": {"name": "Dev", "email": "dev@example.test"}
			}
		]
	}`)

	result, err := service.ProcessWebhook(context.Background(), "push", "delivery-1", body)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "push", result.Event)
	assert.Equal(t, 1, result.Repositories)
	assert.Equal(t, 1, result.Imported)
	assert.Equal(t, 1, result.Linked)
	require.Len(t, repo.upserts, 1)
	assert.Equal(t, []string{ticketID}, repo.linkSets[0])
	assert.NotNil(t, repo.webhookAt)
}

func TestService_ProcessWebhookIgnoresUnsupportedEvent(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakeProjectChecker{}, fakeClient{})

	result, err := service.ProcessWebhook(context.Background(), "issues", "delivery-1", []byte(`{}`))

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Ignored)
	assert.Equal(t, "issues", result.Event)
}

func TestValidGitHubSignature(t *testing.T) {
	body := []byte(`{"zen":"Keep it logically awesome."}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	assert.True(t, validGitHubSignature("secret", signature, body))
	assert.False(t, validGitHubSignature("secret", signature, []byte(`{}`)))
	assert.False(t, validGitHubSignature("wrong", signature, body))
}

func TestTicketReferences(t *testing.T) {
	refs := ticketReferences("Refs #1234abcd, ticket:ABCDEF12 and 11111111-1111-4111-8111-111111111111")

	assert.Equal(t, []string{
		"11111111-1111-4111-8111-111111111111",
		"1234abcd",
		"abcdef12",
	}, refs)
}
