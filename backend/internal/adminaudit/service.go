package adminaudit

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	defaultAuditLimit = 100
	maxAuditLimit     = 500
)

// Service enforces admin authorization before reading audit events.
type Service struct {
	repo Repository
}

// NewService creates an audit service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// ListEvents validates filters and returns audit events for an admin.
func (s *Service) ListEvents(ctx context.Context, adminUserID string, options ListOptions) ([]Event, error) {
	if err := s.requireAdmin(ctx, adminUserID); err != nil {
		return nil, err
	}

	options.Action = strings.TrimSpace(options.Action)
	if options.Action != "" && !IsAction(options.Action) {
		return nil, fmt.Errorf("%w: invalid action", ErrInvalidFilter)
	}

	options.TargetType = strings.TrimSpace(options.TargetType)
	if options.TargetType != "" && !IsTargetType(options.TargetType) {
		return nil, fmt.Errorf("%w: invalid target type", ErrInvalidFilter)
	}

	options.ActorUserID = strings.TrimSpace(options.ActorUserID)
	if options.ActorUserID != "" {
		if _, err := uuid.Parse(options.ActorUserID); err != nil {
			return nil, fmt.Errorf("%w: invalid actor user id", ErrInvalidFilter)
		}
	}

	options.Limit = normalizeLimit(options.Limit)
	options.Offset = normalizeOffset(options.Offset)
	return s.repo.ListEvents(ctx, options)
}

func (s *Service) requireAdmin(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ErrAdminRequired
	}
	role, err := s.repo.UserRole(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrAdminRequired
		}
		return err
	}
	if role != roleAdmin {
		return ErrAdminRequired
	}
	return nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultAuditLimit
	}
	if limit > maxAuditLimit {
		return maxAuditLimit
	}
	return limit
}

func normalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}
