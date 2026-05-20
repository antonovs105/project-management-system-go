package webfinger

import (
	"context"
	"database/sql"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRepository struct {
	actor *ActorResource
	err   error
}

func (m mockRepository) FindLocalActor(ctx context.Context, preferredUsername string) (*ActorResource, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.actor, nil
}

func TestServiceResolve(t *testing.T) {
	cfg := activitypub.NewConfig("https://example.test", "example.test")
	service := NewService(mockRepository{
		actor: &ActorResource{
			Username: "alice",
			Handle:   "alice@example.test",
			APID:     "https://example.test/users/alice",
		},
	}, cfg)

	jrd, err := service.Resolve(context.Background(), "acct:alice@example.test")

	require.NoError(t, err)
	assert.Equal(t, "acct:alice@example.test", jrd.Subject)
	assert.Equal(t, []string{"https://example.test/users/alice"}, jrd.Aliases)
	require.Len(t, jrd.Links, 1)
	assert.Equal(t, "self", jrd.Links[0].Rel)
	assert.Equal(t, "application/activity+json", jrd.Links[0].Type)
	assert.Equal(t, "https://example.test/users/alice", jrd.Links[0].Href)
}

func TestServiceResolveProjectActor(t *testing.T) {
	cfg := activitypub.NewConfig("https://example.test", "example.test")
	service := NewService(mockRepository{
		actor: &ActorResource{
			Username: "project-123",
			Handle:   "project-123@example.test",
			APID:     "https://example.test/projects/123",
		},
	}, cfg)

	jrd, err := service.Resolve(context.Background(), "acct:project-123@example.test")

	require.NoError(t, err)
	assert.Equal(t, "acct:project-123@example.test", jrd.Subject)
	assert.Equal(t, []string{"https://example.test/projects/123"}, jrd.Aliases)
	require.Len(t, jrd.Links, 1)
	assert.Equal(t, "self", jrd.Links[0].Rel)
	assert.Equal(t, "application/activity+json", jrd.Links[0].Type)
	assert.Equal(t, "https://example.test/projects/123", jrd.Links[0].Href)
}

func TestServiceResolveRejectsInvalidResources(t *testing.T) {
	cfg := activitypub.NewConfig("https://example.test", "example.test")
	service := NewService(mockRepository{}, cfg)

	for _, resource := range []string{
		"",
		"alice@example.test",
		"acct:",
		"acct:alice",
		"acct:@example.test",
		"acct:alice@",
		"acct:ali ce@example.test",
		"acct:alice@example test",
	} {
		t.Run(resource, func(t *testing.T) {
			_, err := service.Resolve(context.Background(), resource)
			require.ErrorIs(t, err, ErrInvalidResource)
		})
	}
}

func TestServiceResolveReturnsNotFoundForWrongDomainOrUnknownUser(t *testing.T) {
	cfg := activitypub.NewConfig("https://example.test", "example.test")

	wrongDomain := NewService(mockRepository{}, cfg)
	_, err := wrongDomain.Resolve(context.Background(), "acct:alice@remote.test")
	require.ErrorIs(t, err, ErrNotFound)

	unknownUser := NewService(mockRepository{err: sql.ErrNoRows}, cfg)
	_, err = unknownUser.Resolve(context.Background(), "acct:alice@example.test")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestParseAcctResourceTrimsWhitespaceAndAcceptsCaseInsensitiveScheme(t *testing.T) {
	username, domain, err := parseAcctResource(" AcCt:alice@example.test ")
	require.NoError(t, err)
	assert.Equal(t, "alice", username)
	assert.Equal(t, "example.test", domain)
}
