package delivery

import "context"

type Service struct {
	repo  RecipientRepository
	queue Queue
}

func NewService(repo RecipientRepository, queue Queue) *Service {
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

func (s *Service) EnqueueProjectFollowers(ctx context.Context, projectID string, activityIDs ...string) error {
	if len(activityIDs) == 0 {
		return nil
	}
	inboxes, err := s.repo.RemoteProjectFollowerInboxes(ctx, projectID)
	if err != nil {
		return err
	}
	for _, activityID := range activityIDs {
		if activityID == "" {
			continue
		}
		for _, inbox := range inboxes {
			delivery, _, err := s.repo.Create(ctx, activityID, inbox, DefaultMaxRetry)
			if err != nil {
				return err
			}
			if delivery.State == StateDelivered {
				continue
			}
			if err := s.queue.Enqueue(ctx, delivery.ID, delivery.MaxAttempts); err != nil {
				return err
			}
		}
	}
	return nil
}
