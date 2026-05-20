package moderation

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/domainblock"
)

const (
	defaultInspectionLimit = 100
	maxInspectionLimit     = 500
)

type Service struct {
	repo  Repository
	queue delivery.Queue
}

func NewService(repo Repository, queues ...delivery.Queue) *Service {
	var queue delivery.Queue = delivery.NoopQueue{}
	if len(queues) > 0 && queues[0] != nil {
		queue = queues[0]
	}
	return &Service{repo: repo, queue: queue}
}

func (s *Service) ListDomainBlocks(ctx context.Context, userID string) ([]DomainBlock, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	return s.repo.ListDomainBlocks(ctx)
}

func (s *Service) BlockDomain(ctx context.Context, userID, domain, reason string) (*DomainBlock, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	normalized := domainblock.Normalize(domain)
	if normalized == "" {
		return nil, fmt.Errorf("%w: domain is required", ErrInvalidDomainBlock)
	}
	return s.repo.UpsertDomainBlock(ctx, normalized, strings.TrimSpace(reason), userID)
}

func (s *Service) UnblockDomain(ctx context.Context, userID, domain string) error {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return err
	}
	normalized := domainblock.Normalize(domain)
	if normalized == "" {
		return fmt.Errorf("%w: domain is required", ErrInvalidDomainBlock)
	}
	if err := s.repo.DeleteDomainBlock(ctx, normalized); err != nil {
		if err == sql.ErrNoRows {
			return ErrDomainBlockNotFound
		}
		return err
	}
	return nil
}

func (s *Service) ListRemoteActors(ctx context.Context, userID string, options RemoteActorListOptions) ([]RemoteActorInspection, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	options.Limit = normalizeLimit(options.Limit)
	return s.repo.ListRemoteActors(ctx, options)
}

func (s *Service) ListFederationDeliveries(ctx context.Context, userID string, options FederationDeliveryListOptions) ([]FederationDeliveryInspection, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	if options.State != "" && !delivery.IsDeliveryState(options.State) {
		return nil, fmt.Errorf("%w: invalid delivery state", ErrInvalidFilter)
	}
	if options.FailureKind != "" && !delivery.IsFailureKind(options.FailureKind) {
		return nil, fmt.Errorf("%w: invalid failure kind", ErrInvalidFilter)
	}
	options.Limit = normalizeLimit(options.Limit)
	return s.repo.ListFederationDeliveries(ctx, options)
}

func (s *Service) RetryFederationDelivery(ctx context.Context, userID string, deliveryID string) (*delivery.Delivery, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	delivery, err := s.repo.RetryFederationDelivery(ctx, deliveryID)
	if err != nil {
		return nil, err
	}
	if err := s.queue.Enqueue(ctx, delivery.ID, delivery.MaxAttempts); err != nil {
		return nil, err
	}
	return delivery, nil
}

func (s *Service) requireAdmin(ctx context.Context, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return ErrAdminRequired
	}
	role, err := s.repo.UserRole(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrAdminRequired
		}
		return err
	}
	if role != RoleAdmin {
		return ErrAdminRequired
	}
	return nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultInspectionLimit
	}
	if limit > maxInspectionLimit {
		return maxInspectionLimit
	}
	return limit
}
