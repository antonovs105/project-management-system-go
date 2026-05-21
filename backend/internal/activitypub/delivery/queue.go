package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hibiken/asynq"
)

// Queue enqueues outbound ActivityPub delivery jobs.
type Queue interface {
	Enqueue(ctx context.Context, deliveryID string, maxAttempts int) error
	Close() error
}

// AsynqQueue implements Queue using Redis-backed Asynq tasks.
type AsynqQueue struct {
	client *asynq.Client
}

// NewAsynqQueue creates an Asynq-backed federation delivery queue.
func NewAsynqQueue(redis asynq.RedisConnOpt) *AsynqQueue {
	return &AsynqQueue{client: asynq.NewClient(redis)}
}

// Enqueue adds one delivery task unless an equivalent unique task already exists.
func (q *AsynqQueue) Enqueue(ctx context.Context, deliveryID string, maxAttempts int) error {
	payload, err := json.Marshal(TaskPayload{DeliveryID: deliveryID})
	if err != nil {
		return err
	}
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxRetry
	}

	task := asynq.NewTask(TaskDeliver, payload)
	_, err = q.client.EnqueueContext(
		ctx,
		task,
		asynq.Queue(QueueFederation),
		asynq.MaxRetry(maxAttempts-1),
		asynq.Timeout(30*time.Second),
		asynq.Unique(10*time.Minute),
	)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

// Close releases the underlying Asynq client.
func (q *AsynqQueue) Close() error {
	return q.client.Close()
}

// NoopQueue implements Queue without enqueueing work.
type NoopQueue struct{}

// Enqueue accepts a delivery task without doing anything.
func (NoopQueue) Enqueue(ctx context.Context, deliveryID string, maxAttempts int) error {
	return nil
}

// Close is a no-op for NoopQueue.
func (NoopQueue) Close() error {
	return nil
}
