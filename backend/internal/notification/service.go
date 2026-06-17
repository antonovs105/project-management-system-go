package notification

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/google/uuid"
)

const (
	// defaultListLimit is the default number of notifications returned.
	defaultListLimit = 50
	// maxListLimit caps notification list responses.
	maxListLimit = 100
)

// ErrInvalidInput reports malformed notification API input.
var ErrInvalidInput = errors.New("invalid notification input")

// ErrNotFound reports missing or inaccessible notifications.
var ErrNotFound = errors.New("notification not found")

// Service coordinates notification persistence and realtime fanout.
type Service struct {
	repo   Repository
	events EventPublisher
}

// Option customizes the notification service.
type Option func(*Service)

// WithEventPublisher attaches realtime notification fanout.
func WithEventPublisher(events EventPublisher) Option {
	return func(s *Service) {
		s.events = events
	}
}

// NewService creates a notification service.
func NewService(repo Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

// NotifyTicketAssigned creates a local notification for a ticket assignment.
func (s *Service) NotifyTicketAssigned(ctx context.Context, assigneeID, actorID string, item ticket.Ticket) error {
	if _, err := uuid.Parse(assigneeID); err != nil {
		return ErrInvalidInput
	}
	if _, err := uuid.Parse(actorID); err != nil {
		return ErrInvalidInput
	}
	notification := &Notification{
		UserID:    assigneeID,
		ActorID:   &actorID,
		ProjectID: &item.ProjectID,
		TicketID:  &item.ID,
		Type:      TypeTicketAssigned,
		Title:     "Ticket assigned",
		Body:      fmt.Sprintf("%s was assigned to you.", item.Title),
	}
	if err := s.repo.Create(ctx, notification); err != nil {
		if errors.Is(err, ErrRecipientNotLocal) {
			return nil
		}
		return err
	}
	if s.events != nil {
		s.events.PublishNotification(*notification)
	}
	return nil
}

// List returns notifications for a user.
func (s *Service) List(ctx context.Context, userID string, options ListOptions) ([]Notification, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, ErrInvalidInput
	}
	options.Limit = normalizeLimit(options.Limit)
	if options.Offset < 0 {
		options.Offset = 0
	}
	return s.repo.ListByUserID(ctx, userID, options)
}

// MarkRead marks one notification read.
func (s *Service) MarkRead(ctx context.Context, userID, notificationID string) (*Notification, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, ErrInvalidInput
	}
	if _, err := uuid.Parse(notificationID); err != nil {
		return nil, ErrInvalidInput
	}
	notification, err := s.repo.MarkRead(ctx, userID, notificationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return notification, err
}

// MarkAllRead marks all user notifications read.
func (s *Service) MarkAllRead(ctx context.Context, userID string) error {
	if _, err := uuid.Parse(userID); err != nil {
		return ErrInvalidInput
	}
	return s.repo.MarkAllRead(ctx, userID)
}

// normalizeLimit bounds notification list sizes.
func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}
	if limit > maxListLimit {
		return maxListLimit
	}
	return limit
}
