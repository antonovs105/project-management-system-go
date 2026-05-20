package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hibiken/asynq"
)

type Queue interface {
	Enqueue(ctx context.Context, deliveryID string, maxAttempts int) error
	Close() error
}

type AsynqQueue struct {
	client *asynq.Client
}

func NewAsynqQueue(redis asynq.RedisConnOpt) *AsynqQueue {
	return &AsynqQueue{client: asynq.NewClient(redis)}
}

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

func (q *AsynqQueue) Close() error {
	return q.client.Close()
}

type NoopQueue struct{}

func (NoopQueue) Enqueue(ctx context.Context, deliveryID string, maxAttempts int) error {
	return nil
}

func (NoopQueue) Close() error {
	return nil
}
