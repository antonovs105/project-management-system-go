package federation

import (
	"context"
	"fmt"
	"strings"
)

const (
	// defaultListLimit is the fallback personal federation page size.
	defaultListLimit = 100
	// maxListLimit is the largest personal federation page size.
	maxListLimit = 500
)

// Repository defines persistence operations for personal federation views.
type Repository interface {
	ListInboxActivities(ctx context.Context, userID string, options ListOptions) ([]InboxActivity, error)
	ListRemoteFollows(ctx context.Context, userID string, options ListOptions) ([]RemoteFollow, error)
}

// Service exposes authenticated personal federation views.
type Service struct {
	repo Repository
}

// NewService creates a personal federation service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// ListInboxActivities returns normalized ActivityPub inbox items for the current user.
func (s *Service) ListInboxActivities(ctx context.Context, userID string, options ListOptions) ([]InboxActivity, error) {
	options, err := normalizeListOptions(options, false)
	if err != nil {
		return nil, err
	}
	return s.repo.ListInboxActivities(ctx, userID, options)
}

// ListRemoteFollows returns remote actors followed by the current user.
func (s *Service) ListRemoteFollows(ctx context.Context, userID string, options ListOptions) ([]RemoteFollow, error) {
	options, err := normalizeListOptions(options, true)
	if err != nil {
		return nil, err
	}
	return s.repo.ListRemoteFollows(ctx, userID, options)
}

// normalizeListOptions bounds pagination and validates optional state filters.
func normalizeListOptions(options ListOptions, allowState bool) (ListOptions, error) {
	if options.Limit <= 0 {
		options.Limit = defaultListLimit
	}
	if options.Limit > maxListLimit {
		options.Limit = maxListLimit
	}
	if options.Offset < 0 {
		return ListOptions{}, fmt.Errorf("%w: offset must be non-negative", ErrInvalidFilter)
	}
	options.State = strings.TrimSpace(options.State)
	if options.State != "" {
		if !allowState {
			return ListOptions{}, fmt.Errorf("%w: state filter is not supported", ErrInvalidFilter)
		}
		switch options.State {
		case "pending", "accepted", "rejected":
		default:
			return ListOptions{}, fmt.Errorf("%w: invalid follow state", ErrInvalidFilter)
		}
	}
	return options, nil
}
