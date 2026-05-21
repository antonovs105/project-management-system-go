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
	deleteUserID    string
	deleteErr       error
	actors          []RemoteActorInspection
	actorOptions    RemoteActorListOptions
	deliveries      []FederationDeliveryInspection
	deliveryOptions FederationDeliveryListOptions
	summaryCalled   bool
	summary         *FederationDeliverySummary
	retryDeliveryID string
	retryUserID     string
	retryDelivery   *delivery.Delivery
	retryErr        error
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

func (r *fakeRepository) DeleteDomainBlock(ctx context.Context, domain, userID string) error {
	r.deleteDomain = domain
	r.deleteUserID = userID
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

func (r *fakeRepository) GetFederationDeliverySummary(ctx context.Context) (*FederationDeliverySummary, error) {
	r.summaryCalled = true
	if r.summary != nil {
		return r.summary, nil
	}
	return &FederationDeliverySummary{}, nil
}

func (r *fakeRepository) RetryFederationDelivery(ctx context.Context, deliveryID, userID string) (*delivery.Delivery, error) {
	r.retryDeliveryID = deliveryID
	r.retryUserID = userID
	if r.retryErr != nil {
		return nil, r.retryErr
	}
	if r.retryDelivery != nil {
		return r.retryDelivery, nil
	}
	return &delivery.Delivery{ID: deliveryID, MaxAttempts: delivery.DefaultMaxRetry, State: delivery.StatePending}, nil
}

type fakeQueue struct {
	deliveryID  string
	maxAttempts int
	err         error
}

func (q *fakeQueue) Enqueue(ctx context.Context, deliveryID string, maxAttempts int) error {
	q.deliveryID = deliveryID
	q.maxAttempts = maxAttempts
	return q.err
}

func (q *fakeQueue) Close() error {
	return nil
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

func TestServiceUnblockDomainRecordsAdmin(t *testing.T) {
	repo := &fakeRepository{role: RoleAdmin}
	service := NewService(repo)

	err := service.UnblockDomain(context.Background(), "admin-1", "remote.example")

	require.NoError(t, err)
	assert.Equal(t, "remote.example", repo.deleteDomain)
	assert.Equal(t, "admin-1", repo.deleteUserID)
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

func TestServiceGetsFederationDeliverySummaryForAdmin(t *testing.T) {
	repo := &fakeRepository{role: RoleAdmin, summary: &FederationDeliverySummary{Total: 3, Dead: 1, Retryable: 1, CanRetry: true}}
	service := NewService(repo)

	summary, err := service.GetFederationDeliverySummary(context.Background(), "admin-1")

	require.NoError(t, err)
	assert.True(t, repo.summaryCalled)
	assert.Equal(t, 3, summary.Total)
	assert.Equal(t, 1, summary.Dead)
	assert.True(t, summary.CanRetry)
}

func TestServiceFederationDeliverySummaryRequiresAdmin(t *testing.T) {
	repo := &fakeRepository{role: "worker"}
	service := NewService(repo)

	_, err := service.GetFederationDeliverySummary(context.Background(), "user-1")

	require.ErrorIs(t, err, ErrAdminRequired)
	assert.False(t, repo.summaryCalled)
}

func TestServiceRetryFederationDeliveryQueuesTask(t *testing.T) {
	repo := &fakeRepository{role: RoleAdmin}
	queue := &fakeQueue{}
	service := NewService(repo, queue)

	retried, err := service.RetryFederationDelivery(context.Background(), "admin-1", "delivery-1")

	require.NoError(t, err)
	assert.Equal(t, "delivery-1", retried.ID)
	assert.Equal(t, "delivery-1", repo.retryDeliveryID)
	assert.Equal(t, "admin-1", repo.retryUserID)
	assert.Equal(t, "delivery-1", queue.deliveryID)
	assert.Equal(t, delivery.DefaultMaxRetry, queue.maxAttempts)
}

func TestServiceRetryFederationDeliveryRequiresAdmin(t *testing.T) {
	repo := &fakeRepository{role: "worker"}
	queue := &fakeQueue{}
	service := NewService(repo, queue)

	_, err := service.RetryFederationDelivery(context.Background(), "user-1", "delivery-1")

	require.ErrorIs(t, err, ErrAdminRequired)
	assert.Empty(t, repo.retryDeliveryID)
	assert.Empty(t, queue.deliveryID)
}
