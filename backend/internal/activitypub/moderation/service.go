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
	// defaultInspectionLimit is the fallback admin federation inspection page size.
	defaultInspectionLimit = 100
	// maxInspectionLimit is the largest admin federation inspection page size.
	maxInspectionLimit = 500
)

// Service enforces admin access for federation moderation workflows.
type Service struct {
	repo  Repository
	queue delivery.Queue
}

// NewService creates a federation moderation service.
func NewService(repo Repository, queues ...delivery.Queue) *Service {
	var queue delivery.Queue = delivery.NoopQueue{}
	if len(queues) > 0 && queues[0] != nil {
		queue = queues[0]
	}
	return &Service{repo: repo, queue: queue}
}

// ListDomainBlocks returns configured domain blocks for an admin.
func (s *Service) ListDomainBlocks(ctx context.Context, userID string) ([]DomainBlock, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	return s.repo.ListDomainBlocks(ctx)
}

// BlockDomain creates or updates a federation domain block.
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

// UnblockDomain removes a federation domain block.
func (s *Service) UnblockDomain(ctx context.Context, userID, domain string) error {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return err
	}
	normalized := domainblock.Normalize(domain)
	if normalized == "" {
		return fmt.Errorf("%w: domain is required", ErrInvalidDomainBlock)
	}
	if err := s.repo.DeleteDomainBlock(ctx, normalized, userID); err != nil {
		if err == sql.ErrNoRows {
			return ErrDomainBlockNotFound
		}
		return err
	}
	return nil
}

// ListRemoteActors returns cached remote actors for admin inspection.
func (s *Service) ListRemoteActors(ctx context.Context, userID string, options RemoteActorListOptions) ([]RemoteActorInspection, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	options.Limit = normalizeLimit(options.Limit)
	return s.repo.ListRemoteActors(ctx, options)
}

// ListFederationDeliveries returns outbound deliveries for admin inspection.
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

// GetFederationDeliverySummary returns global delivery health for an admin.
func (s *Service) GetFederationDeliverySummary(ctx context.Context, userID string) (*FederationDeliverySummary, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	return s.repo.GetFederationDeliverySummary(ctx)
}

// RetryFederationDelivery resets and requeues an outbound delivery.
func (s *Service) RetryFederationDelivery(ctx context.Context, userID string, deliveryID string) (*delivery.Delivery, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	delivery, err := s.repo.RetryFederationDelivery(ctx, deliveryID, userID)
	if err != nil {
		return nil, err
	}
	if err := s.queue.Enqueue(ctx, delivery.ID, delivery.MaxAttempts); err != nil {
		return nil, err
	}
	return delivery, nil
}

// requireAdmin verifies that a user has instance admin privileges.
func (s *Service) requireAdmin(ctx context.Context, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return ErrAdminRequired
	}
	role, err := s.repo.InstanceRole(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrAdminRequired
		}
		return err
	}
	if role != InstanceRoleOwner && role != InstanceRoleAdmin {
		return ErrAdminRequired
	}
	return nil
}

// normalizeLimit bounds admin federation inspection list sizes.
func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultInspectionLimit
	}
	if limit > maxInspectionLimit {
		return maxInspectionLimit
	}
	return limit
}
