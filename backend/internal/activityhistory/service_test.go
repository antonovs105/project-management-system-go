package activityhistory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// permissionStub controls activity permission checks.
type permissionStub bool

// HasPermission returns the configured decision.
func (p permissionStub) HasPermission(context.Context, string, string, string) (bool, error) {
	return bool(p), nil
}

// eventRepositoryStub captures normalized pagination.
type eventRepositoryStub struct{ limit, offset int }

// List captures inputs and returns an empty page.
func (r *eventRepositoryStub) List(_ context.Context, _ string, limit, offset int) ([]Event, error) {
	r.limit, r.offset = limit, offset
	return []Event{}, nil
}

func TestActivityServiceAuthorizesAndBoundsPagination(t *testing.T) {
	denied := NewService(&eventRepositoryStub{}, permissionStub(false))
	_, err := denied.List(context.Background(), "project", "user", 50, 0)
	require.ErrorIs(t, err, ErrForbidden)

	repository := &eventRepositoryStub{}
	allowed := NewService(repository, permissionStub(true))
	_, err = allowed.List(context.Background(), "project", "user", 1000, -5)
	require.NoError(t, err)
	require.Equal(t, 200, repository.limit)
	require.Zero(t, repository.offset)
}
