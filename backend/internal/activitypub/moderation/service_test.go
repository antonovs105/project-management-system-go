package moderation

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	role            string
	roleErr         error
	blocks          []DomainBlock
	upsertDomain    string
	upsertReason    string
	upsertUserID    string
	deleteDomain    string
	deleteErr       error
	actors          []RemoteActorInspection
	actorOptions    RemoteActorListOptions
	deliveries      []FederationDeliveryInspection
	deliveryOptions FederationDeliveryListOptions
}

func (r *fakeRepository) UserRole(ctx context.Context, userID string) (string, error) {
	if r.roleErr != nil {
		return "", r.roleErr
	}
	return r.role, nil
}

func (r *fakeRepository) ListDomainBlocks(ctx context.Context) ([]DomainBlock, error) {
	return r.blocks, nil
}

func (r *fakeRepository) UpsertDomainBlock(ctx context.Context, domain, reason, userID string) (*DomainBlock, error) {
	r.upsertDomain = domain
	r.upsertReason = reason
	r.upsertUserID = userID
	return &DomainBlock{
		ID:        "block-1",
		Domain:    domain,
		Reason:    reason,
		CreatedBy: &userID,
		CreatedAt: time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (r *fakeRepository) DeleteDomainBlock(ctx context.Context, domain string) error {
	r.deleteDomain = domain
	return r.deleteErr
}

func (r *fakeRepository) ListRemoteActors(ctx context.Context, options RemoteActorListOptions) ([]RemoteActorInspection, error) {
	r.actorOptions = options
	return r.actors, nil
}

func (r *fakeRepository) ListFederationDeliveries(ctx context.Context, options FederationDeliveryListOptions) ([]FederationDeliveryInspection, error) {
	r.deliveryOptions = options
	return r.deliveries, nil
}

func TestServiceBlockDomainRequiresAdmin(t *testing.T) {
	repo := &fakeRepository{role: "worker"}
	service := NewService(repo)

	_, err := service.BlockDomain(context.Background(), "user-1", "remote.example", "")

	require.ErrorIs(t, err, ErrAdminRequired)
	assert.Empty(t, repo.upsertDomain)
}

func TestServiceBlockDomainNormalizesDomain(t *testing.T) {
	repo := &fakeRepository{role: RoleAdmin}
	service := NewService(repo)

	block, err := service.BlockDomain(context.Background(), "admin-1", "HTTPS://Remote.Example/users/alice", " spam ")

	require.NoError(t, err)
	assert.Equal(t, "remote.example", block.Domain)
	assert.Equal(t, "remote.example", repo.upsertDomain)
	assert.Equal(t, "spam", repo.upsertReason)
	assert.Equal(t, "admin-1", repo.upsertUserID)
}

func TestServiceRejectsInvalidDomain(t *testing.T) {
	service := NewService(&fakeRepository{role: RoleAdmin})

	_, err := service.BlockDomain(context.Background(), "admin-1", "bad/domain", "")

	require.ErrorIs(t, err, ErrInvalidDomainBlock)
}

func TestServiceUnblockMapsMissingDomain(t *testing.T) {
	service := NewService(&fakeRepository{role: RoleAdmin, deleteErr: sql.ErrNoRows})

	err := service.UnblockDomain(context.Background(), "admin-1", "remote.example")

	require.ErrorIs(t, err, ErrDomainBlockNotFound)
}

func TestServiceListRemoteActorsUsesAdminAndOptions(t *testing.T) {
	repo := &fakeRepository{role: RoleAdmin}
	service := NewService(repo)

	_, err := service.ListRemoteActors(context.Background(), "admin-1", RemoteActorListOptions{
		FetchErrorOnly: true,
		Limit:          1000,
	})

	require.NoError(t, err)
	assert.True(t, repo.actorOptions.FetchErrorOnly)
	assert.Equal(t, maxInspectionLimit, repo.actorOptions.Limit)
}

func TestServiceListFederationDeliveriesValidatesFilters(t *testing.T) {
	repo := &fakeRepository{role: RoleAdmin}
	service := NewService(repo)

	_, err := service.ListFederationDeliveries(context.Background(), "admin-1", FederationDeliveryListOptions{
		State:       delivery.StateDead,
		FailureKind: delivery.FailureKindHTTP,
		Limit:       25,
	})

	require.NoError(t, err)
	assert.Equal(t, delivery.StateDead, repo.deliveryOptions.State)
	assert.Equal(t, delivery.FailureKindHTTP, repo.deliveryOptions.FailureKind)
	assert.Equal(t, 25, repo.deliveryOptions.Limit)

	_, err = service.ListFederationDeliveries(context.Background(), "admin-1", FederationDeliveryListOptions{FailureKind: "weird"})
	require.ErrorIs(t, err, ErrInvalidFilter)
}
