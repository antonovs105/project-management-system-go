package federation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type serviceRepo struct {
	inboxOptions  ListOptions
	followOptions ListOptions
}

func (r *serviceRepo) ListInboxActivities(ctx context.Context, userID string, options ListOptions) ([]InboxActivity, error) {
	r.inboxOptions = options
	return []InboxActivity{{ID: "activity-1"}}, nil
}

func (r *serviceRepo) ListRemoteFollows(ctx context.Context, userID string, options ListOptions) ([]RemoteFollow, error) {
	r.followOptions = options
	return []RemoteFollow{{ActorID: "actor-1", State: options.State}}, nil
}

func TestServiceNormalizesPersonalFederationListLimits(t *testing.T) {
	repo := &serviceRepo{}
	service := NewService(repo)

	activities, err := service.ListInboxActivities(context.Background(), "user-1", ListOptions{Limit: 999, Offset: 2})

	require.NoError(t, err)
	require.Len(t, activities, 1)
	assert.Equal(t, maxListLimit, repo.inboxOptions.Limit)
	assert.Equal(t, 2, repo.inboxOptions.Offset)
}

func TestServiceFiltersRemoteFollowsByState(t *testing.T) {
	repo := &serviceRepo{}
	service := NewService(repo)

	follows, err := service.ListRemoteFollows(context.Background(), "user-1", ListOptions{State: "accepted"})

	require.NoError(t, err)
	require.Len(t, follows, 1)
	assert.Equal(t, "accepted", repo.followOptions.State)
	assert.Equal(t, defaultListLimit, repo.followOptions.Limit)
}

func TestServiceRejectsInvalidPersonalFederationFilters(t *testing.T) {
	repo := &serviceRepo{}
	service := NewService(repo)

	_, err := service.ListInboxActivities(context.Background(), "user-1", ListOptions{Offset: -1})
	require.ErrorIs(t, err, ErrInvalidFilter)

	_, err = service.ListInboxActivities(context.Background(), "user-1", ListOptions{State: "accepted"})
	require.ErrorIs(t, err, ErrInvalidFilter)

	_, err = service.ListRemoteFollows(context.Background(), "user-1", ListOptions{State: "blocked"})
	require.ErrorIs(t, err, ErrInvalidFilter)
}
