package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type serviceRepo struct {
	activityID        string
	targetInboxURL    string
	maxAttempts       int
	inboxes           []string
	ticketInboxes     []string
	projectDeliveries []ProjectDelivery
	projectID         string
	userID            string
	ticketID          string
	retryDeliveryID   string
	delivery          *Delivery
	retryDelivery     *Delivery
	created           []*Delivery
	err               error
	retryErr          error
}

func (r *serviceRepo) Create(ctx context.Context, activityID string, targetInboxURL string, maxAttempts int) (*Delivery, bool, error) {
	r.activityID = activityID
	r.targetInboxURL = targetInboxURL
	r.maxAttempts = maxAttempts
	if r.err != nil {
		return nil, false, r.err
	}
	if r.delivery != nil {
		return r.delivery, true, nil
	}
	delivery := &Delivery{ID: "delivery-" + targetInboxURL, MaxAttempts: maxAttempts, State: StatePending}
	r.created = append(r.created, delivery)
	return delivery, true, nil
}

func (r *serviceRepo) StartAttempt(ctx context.Context, deliveryID string) (*Delivery, error) {
	return nil, nil
}

func (r *serviceRepo) MarkDelivered(ctx context.Context, deliveryID string) error {
	return nil
}

func (r *serviceRepo) MarkFailed(ctx context.Context, deliveryID string, message string, nextAttemptAt *time.Time) error {
	return nil
}

func (r *serviceRepo) ProjectDeliveries(ctx context.Context, projectID string, userID string) ([]ProjectDelivery, error) {
	if r.err != nil {
		return nil, r.err
	}
	r.projectID = projectID
	r.userID = userID
	return r.projectDeliveries, nil
}

func (r *serviceRepo) RetryProjectDelivery(ctx context.Context, projectID string, userID string, deliveryID string) (*Delivery, error) {
	r.projectID = projectID
	r.userID = userID
	r.retryDeliveryID = deliveryID
	if r.retryErr != nil {
		return nil, r.retryErr
	}
	if r.retryDelivery != nil {
		return r.retryDelivery, nil
	}
	return &Delivery{ID: deliveryID, State: StatePending, MaxAttempts: DefaultMaxRetry}, nil
}

func (r *serviceRepo) RemoteProjectFollowerInboxes(ctx context.Context, projectID string) ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	r.projectID = projectID
	return r.inboxes, nil
}

func (r *serviceRepo) RemoteProjectTicketRecipientInboxes(ctx context.Context, projectID string, ticketID string) ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	r.projectID = projectID
	r.ticketID = ticketID
	return r.ticketInboxes, nil
}

type serviceQueue struct {
	deliveryID  string
	maxAttempts int
	err         error
}

func (q *serviceQueue) Enqueue(ctx context.Context, deliveryID string, maxAttempts int) error {
	q.deliveryID = deliveryID
	q.maxAttempts = maxAttempts
	return q.err
}

func (q *serviceQueue) Close() error {
	return nil
}

func TestServiceEnqueueCreatesDeliveryAndQueuesTask(t *testing.T) {
	repo := &serviceRepo{delivery: &Delivery{ID: "delivery-1", MaxAttempts: 10}}
	queue := &serviceQueue{}
	service := NewService(repo, queue)

	delivery, err := service.Enqueue(context.Background(), "activity-1", "https://remote.example/inbox")

	require.NoError(t, err)
	assert.Equal(t, "delivery-1", delivery.ID)
	assert.Equal(t, "activity-1", repo.activityID)
	assert.Equal(t, "https://remote.example/inbox", repo.targetInboxURL)
	assert.Equal(t, DefaultMaxRetry, repo.maxAttempts)
	assert.Equal(t, "delivery-1", queue.deliveryID)
	assert.Equal(t, 10, queue.maxAttempts)
}

func TestServiceEnqueueSkipsTerminalDelivery(t *testing.T) {
	repo := &serviceRepo{delivery: &Delivery{ID: "delivery-1", MaxAttempts: 10, State: StateDead}}
	queue := &serviceQueue{}
	service := NewService(repo, queue)

	delivery, err := service.Enqueue(context.Background(), "activity-1", "https://remote.example/inbox")

	require.NoError(t, err)
	assert.Equal(t, "delivery-1", delivery.ID)
	assert.Empty(t, queue.deliveryID)
}

func TestServiceEnqueueProjectFollowersCreatesDeliveriesForRemoteInboxes(t *testing.T) {
	repo := &serviceRepo{inboxes: []string{"https://remote.example/alice/inbox", "https://remote.example/bob/inbox"}}
	queue := &serviceQueue{}
	service := NewService(repo, queue)

	err := service.EnqueueProjectFollowers(context.Background(), "project-1", "activity-1")

	require.NoError(t, err)
	assert.Len(t, repo.created, 2)
	assert.Equal(t, "activity-1", repo.activityID)
	assert.Equal(t, "https://remote.example/bob/inbox", repo.targetInboxURL)
	assert.Equal(t, repo.created[1].ID, queue.deliveryID)
}

func TestServiceEnqueueProjectTicketRecipientsCreatesDeliveriesForRemoteInboxes(t *testing.T) {
	repo := &serviceRepo{ticketInboxes: []string{"https://remote.example/alice/inbox", "https://remote.example/thread/inbox"}}
	queue := &serviceQueue{}
	service := NewService(repo, queue)

	err := service.EnqueueProjectTicketRecipients(context.Background(), "project-1", "ticket-1", "activity-1")

	require.NoError(t, err)
	assert.Len(t, repo.created, 2)
	assert.Equal(t, "project-1", repo.projectID)
	assert.Equal(t, "ticket-1", repo.ticketID)
	assert.Equal(t, "activity-1", repo.activityID)
	assert.Equal(t, "https://remote.example/thread/inbox", repo.targetInboxURL)
}

func TestServiceListProjectDeliveriesUsesProjectAndUserScope(t *testing.T) {
	repo := &serviceRepo{
		projectDeliveries: []ProjectDelivery{
			{
				ID:             "delivery-1",
				ActivityAPID:   "https://local.example/activities/1",
				ActivityType:   "Create",
				TargetInboxURL: "https://remote.example/inbox",
				State:          StatePending,
				Attempts:       1,
				MaxAttempts:    10,
			},
		},
	}
	service := NewService(repo, &serviceQueue{})

	deliveries, err := service.ListProjectDeliveries(context.Background(), "project-1", "user-1")

	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, "project-1", repo.projectID)
	assert.Equal(t, "user-1", repo.userID)
	assert.Equal(t, "https://local.example/activities/1", deliveries[0].ActivityAPID)
}

func TestServiceRetryProjectDeliveryResetsAndQueuesTask(t *testing.T) {
	repo := &serviceRepo{
		retryDelivery: &Delivery{ID: "delivery-1", State: StatePending, MaxAttempts: DefaultMaxRetry},
	}
	queue := &serviceQueue{}
	service := NewService(repo, queue)

	delivery, err := service.RetryProjectDelivery(context.Background(), "project-1", "user-1", "delivery-1")

	require.NoError(t, err)
	assert.Equal(t, "project-1", repo.projectID)
	assert.Equal(t, "user-1", repo.userID)
	assert.Equal(t, "delivery-1", repo.retryDeliveryID)
	assert.Equal(t, StatePending, delivery.State)
	assert.Equal(t, "delivery-1", queue.deliveryID)
	assert.Equal(t, DefaultMaxRetry, queue.maxAttempts)
}

func TestServiceRetryProjectDeliveryReturnsRepositoryError(t *testing.T) {
	repo := &serviceRepo{retryErr: ErrDeliveryRetryDenied}
	queue := &serviceQueue{}
	service := NewService(repo, queue)

	_, err := service.RetryProjectDelivery(context.Background(), "project-1", "viewer-1", "delivery-1")

	require.ErrorIs(t, err, ErrDeliveryRetryDenied)
	assert.Empty(t, queue.deliveryID)
}

func TestServiceRetryProjectDeliveryReturnsQueueError(t *testing.T) {
	repo := &serviceRepo{}
	queue := &serviceQueue{err: assert.AnError}
	service := NewService(repo, queue)

	_, err := service.RetryProjectDelivery(context.Background(), "project-1", "user-1", "delivery-1")

	require.ErrorIs(t, err, assert.AnError)
}
