package moderation

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/domainblock"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
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
