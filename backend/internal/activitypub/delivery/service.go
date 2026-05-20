package delivery

import "context"

type Service struct {
	repo  Repository
	queue Queue
}

func NewService(repo Repository, queue Queue) *Service {
	if queue == nil {
		queue = NoopQueue{}
	}
	return &Service{repo: repo, queue: queue}
}

func (s *Service) Enqueue(ctx context.Context, activityID string, targetInboxURL string) (*Delivery, error) {
	delivery, _, err := s.repo.Create(ctx, activityID, targetInboxURL, DefaultMaxRetry)
	if err != nil {
		return nil, err
	}
	if err := s.queue.Enqueue(ctx, delivery.ID, delivery.MaxAttempts); err != nil {
		return nil, err
	}
	return delivery, nil
}
