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
	delivery       *Delivery
	err            error
}

func (r *serviceRepo) Create(ctx context.Context, activityID string, targetInboxURL string, maxAttempts int) (*Delivery, bool, error) {
	r.activityID = activityID
	r.targetInboxURL = targetInboxURL
	r.maxAttempts = maxAttempts
	if r.err != nil {
		return nil, false, r.err
	}
	return r.delivery, true, nil
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
