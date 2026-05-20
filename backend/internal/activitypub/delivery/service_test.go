package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type serviceRepo struct {
	activityID     string
	targetInboxURL string
	maxAttempts    int
	inboxes        []string
	ticketInboxes  []string
	projectID      string
	ticketID       string
	delivery       *Delivery
	created        []*Delivery
	err            error
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
