package delivery

import "context"

// Service creates delivery rows and queues outbound federation work.
type Service struct {
	repo  RecipientRepository
	queue Queue
}

// NewService creates an outbound delivery service.
func NewService(repo RecipientRepository, queue Queue) *Service {
	if queue == nil {
		queue = NoopQueue{}
	}
	return &Service{repo: repo, queue: queue}
}

// Enqueue creates or reuses a delivery for an activity and inbox.
func (s *Service) Enqueue(ctx context.Context, activityID string, targetInboxURL string) (*Delivery, error) {
	delivery, _, err := s.repo.Create(ctx, activityID, targetInboxURL, DefaultMaxRetry)
	if err != nil {
		return nil, err
	}
	return s.queueDelivery(ctx, delivery)
}

// EnqueueWithActor creates a delivery using an explicit signing actor.
func (s *Service) EnqueueWithActor(ctx context.Context, activityID string, actorID string, targetInboxURL string) (*Delivery, error) {
	delivery, _, err := s.repo.CreateWithActor(ctx, activityID, actorID, targetInboxURL, DefaultMaxRetry)
	if err != nil {
		return nil, err
	}
	return s.queueDelivery(ctx, delivery)
}

func (s *Service) queueDelivery(ctx context.Context, delivery *Delivery) (*Delivery, error) {
	if delivery.State != StateDelivered && delivery.State != StateDead {
		if err := s.queue.Enqueue(ctx, delivery.ID, delivery.MaxAttempts); err != nil {
			return nil, err
		}
	}
	return delivery, nil
}

// ListProjectDeliveries returns project deliveries with default filters.
func (s *Service) ListProjectDeliveries(ctx context.Context, projectID string, userID string) ([]ProjectDelivery, error) {
	return s.ListProjectDeliveriesWithOptions(ctx, projectID, userID, ProjectDeliveryListOptions{})
}

// ListProjectDeliveriesWithOptions returns project deliveries matching filters.
func (s *Service) ListProjectDeliveriesWithOptions(ctx context.Context, projectID string, userID string, options ProjectDeliveryListOptions) ([]ProjectDelivery, error) {
	options, err := NormalizeProjectDeliveryListOptions(options)
	if err != nil {
		return nil, err
	}
	return s.repo.ProjectDeliveries(ctx, projectID, userID, options)
}

// GetProjectDeliverySummary returns aggregate delivery state counts for a project.
func (s *Service) GetProjectDeliverySummary(ctx context.Context, projectID string, userID string) (*ProjectDeliverySummary, error) {
	return s.repo.ProjectDeliverySummary(ctx, projectID, userID)
}

// RetryProjectDelivery resets and requeues a retryable project delivery.
func (s *Service) RetryProjectDelivery(ctx context.Context, projectID string, userID string, deliveryID string) (*Delivery, error) {
	delivery, err := s.repo.RetryProjectDelivery(ctx, projectID, userID, deliveryID)
	if err != nil {
		return nil, err
	}
	if err := s.queue.Enqueue(ctx, delivery.ID, delivery.MaxAttempts); err != nil {
		return nil, err
	}
	return delivery, nil
}

// EnqueueProjectFollowers queues activities for all remote project followers.
func (s *Service) EnqueueProjectFollowers(ctx context.Context, projectID string, activityIDs ...string) error {
	if len(activityIDs) == 0 {
		return nil
	}
	inboxes, err := s.repo.RemoteProjectFollowerInboxes(ctx, projectID)
	if err != nil {
		return err
	}
	return s.enqueueToInboxes(ctx, inboxes, activityIDs...)
}

// EnqueueProjectTicketRecipients queues activities for remote recipients related to a ticket.
func (s *Service) EnqueueProjectTicketRecipients(ctx context.Context, projectID string, ticketID string, activityIDs ...string) error {
	if len(activityIDs) == 0 {
		return nil
	}
	inboxes, err := s.repo.RemoteProjectTicketRecipientInboxes(ctx, projectID, ticketID)
	if err != nil {
		return err
	}
	return s.enqueueToInboxes(ctx, inboxes, activityIDs...)
}

func (s *Service) enqueueToInboxes(ctx context.Context, inboxes []string, activityIDs ...string) error {
	for _, activityID := range activityIDs {
		if activityID == "" {
			continue
		}
		for _, inbox := range inboxes {
			delivery, _, err := s.repo.Create(ctx, activityID, inbox, DefaultMaxRetry)
			if err != nil {
				return err
			}
			if delivery.State == StateDelivered || delivery.State == StateDead {
				continue
			}
			if err := s.queue.Enqueue(ctx, delivery.ID, delivery.MaxAttempts); err != nil {
				return err
			}
		}
	}
	return nil
}
