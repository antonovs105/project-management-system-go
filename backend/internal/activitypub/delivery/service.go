package delivery

import (
	"context"
	"log"
	"time"
)

const (
	// defaultRecoveryInterval controls how often missing delivery tasks are repaired.
	defaultRecoveryInterval = 30 * time.Second
	// defaultRecoveryLimit bounds one delivery recovery scan.
	defaultRecoveryLimit = 100
)

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

// queueDelivery sends a delivery task unless the row is already terminal.
func (s *Service) queueDelivery(ctx context.Context, delivery *Delivery) (*Delivery, error) {
	if delivery.State != StateDelivered && delivery.State != StateDead {
		if err := s.queue.Enqueue(ctx, delivery.ID, delivery.MaxAttempts); err != nil {
			return nil, err
		}
		log.Printf(
			"activitypub_delivery_queued delivery_id=%s activity_id=%s activity_ap_id=%s target_inbox_url=%s state=%s max_attempts=%d",
			delivery.ID,
			delivery.ActivityID,
			delivery.ActivityAPID,
			delivery.TargetInboxURL,
			delivery.State,
			delivery.MaxAttempts,
		)
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
	if err := s.EnqueuePersisted(ctx, []QueueCandidate{{ID: delivery.ID, MaxAttempts: delivery.MaxAttempts}}); err != nil {
		return nil, err
	}
	return delivery, nil
}

// EnqueuePersisted queues delivery rows that were already committed transactionally.
func (s *Service) EnqueuePersisted(ctx context.Context, deliveries []QueueCandidate) error {
	for _, delivery := range deliveries {
		if delivery.ID == "" {
			continue
		}
		maxAttempts := delivery.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = DefaultMaxRetry
		}
		if err := s.queue.Enqueue(ctx, delivery.ID, maxAttempts); err != nil {
			return err
		}
	}
	return nil
}

// RecoverDueDeliveries re-enqueues persisted delivery rows that have no guaranteed live task.
func (s *Service) RecoverDueDeliveries(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = defaultRecoveryLimit
	}
	deliveries, err := s.repo.DueDeliveries(ctx, limit)
	if err != nil {
		return 0, err
	}
	recovered := 0
	for _, delivery := range deliveries {
		if err := s.EnqueuePersisted(ctx, []QueueCandidate{delivery}); err != nil {
			return recovered, err
		}
		recovered++
	}
	return recovered, nil
}

// StartRecoveryLoop periodically re-enqueues due delivery rows.
func (s *Service) StartRecoveryLoop(parent context.Context, interval time.Duration, limit int) func() {
	if interval <= 0 {
		interval = defaultRecoveryInterval
	}
	ctx, cancel := context.WithCancel(parent)
	go func() {
		s.recoverAndLog(ctx, limit)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.recoverAndLog(ctx, limit)
			}
		}
	}()
	return cancel
}

// recoverAndLog runs one delivery recovery pass and writes structured logs.
func (s *Service) recoverAndLog(ctx context.Context, limit int) {
	recovered, err := s.RecoverDueDeliveries(ctx, limit)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("activitypub_delivery_recovery_failed error=%v", err)
		}
		return
	}
	if recovered > 0 {
		log.Printf("activitypub_delivery_recovery_enqueued count=%d", recovered)
	}
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

// enqueueToInboxes creates delivery rows for activities across remote inboxes.
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
			if _, err := s.queueDelivery(ctx, delivery); err != nil {
				return err
			}
		}
	}
	return nil
}
