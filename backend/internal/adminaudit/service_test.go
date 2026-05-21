package adminaudit

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	role    string
	roleErr error
	options ListOptions
	events  []Event
}

func (r *fakeRepository) InstanceRole(ctx context.Context, userID string) (string, error) {
	if r.roleErr != nil {
		return "", r.roleErr
	}
	return r.role, nil
}

func (r *fakeRepository) ListEvents(ctx context.Context, options ListOptions) ([]Event, error) {
	r.options = options
	return r.events, nil
}

func TestServiceListEventsRequiresAdmin(t *testing.T) {
	service := NewService(&fakeRepository{role: "user"})

	events, err := service.ListEvents(context.Background(), "user-1", ListOptions{})

	require.ErrorIs(t, err, ErrAdminRequired)
	assert.Nil(t, events)
}

func TestServiceListEventsTreatsMissingUserAsForbidden(t *testing.T) {
	service := NewService(&fakeRepository{roleErr: sql.ErrNoRows})

	events, err := service.ListEvents(context.Background(), "missing-user", ListOptions{})

	require.ErrorIs(t, err, ErrAdminRequired)
	assert.Nil(t, events)
}

func TestServiceListEventsValidatesFilters(t *testing.T) {
	repo := &fakeRepository{role: instanceRoleAdmin}
	service := NewService(repo)

	events, err := service.ListEvents(context.Background(), "admin-1", ListOptions{
		Action:      ActionFederationDomainBlocked,
		TargetType:  TargetTypeFederationDomain,
		ActorUserID: "11111111-1111-4111-8111-111111111111",
		Limit:       1000,
		Offset:      -10,
	})

	require.NoError(t, err)
	assert.Empty(t, events)
	assert.Equal(t, ActionFederationDomainBlocked, repo.options.Action)
	assert.Equal(t, TargetTypeFederationDomain, repo.options.TargetType)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", repo.options.ActorUserID)
	assert.Equal(t, maxAuditLimit, repo.options.Limit)
	assert.Zero(t, repo.options.Offset)

	_, err = service.ListEvents(context.Background(), "admin-1", ListOptions{Action: "weird"})
	require.ErrorIs(t, err, ErrInvalidFilter)

	_, err = service.ListEvents(context.Background(), "admin-1", ListOptions{TargetType: "project"})
	require.ErrorIs(t, err, ErrInvalidFilter)

	_, err = service.ListEvents(context.Background(), "admin-1", ListOptions{ActorUserID: "not-a-uuid"})
	require.ErrorIs(t, err, ErrInvalidFilter)
}
