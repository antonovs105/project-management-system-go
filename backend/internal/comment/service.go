package comment

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/apperror"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
)

// ErrInvalidCommentInput reports malformed comment input.
var ErrInvalidCommentInput = errors.New("invalid comment input")

const (
	// defaultCommentListLimit is the fallback comment list size.
	defaultCommentListLimit = 100
	// maxCommentListLimit is the largest accepted comment list size.
	maxCommentListLimit = 500
	// maxCommentContentLength bounds local notes and imported comment payloads.
	maxCommentContentLength = 20000
)

// TicketChecker exposes ticket lookups and project roles needed by comments.
type TicketChecker interface {
	GetTicketByID(ctx context.Context, ticketID, userID string) (*ticket.Ticket, error)
	HasProjectPermission(ctx context.Context, projectID, userID, permission string) (bool, error)
}

// Service contains comment workflows and ActivityPub side effects.
type Service struct {
	repo          Repository
	tickets       TicketChecker
	apConfig      activitypub.Config
	delivery      DeliveryEnqueuer
	notifications NotificationSink
}

// NewService creates a comment service.
func NewService(repo Repository, tickets TicketChecker, apConfig activitypub.Config) *Service {
	return &Service{repo: repo, tickets: tickets, apConfig: apConfig}
}

// DeliveryEnqueuer queues federation deliveries created by comment actions.
type DeliveryEnqueuer interface {
	EnqueuePersisted(ctx context.Context, deliveries []apdelivery.QueueCandidate) error
}

// NotificationSink receives local notifications caused by comment workflows.
type NotificationSink interface {
	NotifyCommentCreated(ctx context.Context, actorID string, ticket ticket.Ticket, content string) error
}

// SetDelivery attaches the delivery queue used for comment federation.
func (s *Service) SetDelivery(delivery DeliveryEnqueuer) {
	s.delivery = delivery
}

// SetNotificationSink attaches local comment and mention notifications.
func (s *Service) SetNotificationSink(notifications NotificationSink) {
	s.notifications = notifications
}

// CreateComment creates a Note on a ticket and records its Create activity.
func (s *Service) CreateComment(ctx context.Context, ticketID, authorID, content string) (*Comment, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, invalidCommentInput("content is required")
	}
	if utf8.RuneCountInString(content) > maxCommentContentLength {
		return nil, invalidCommentInput(fmt.Sprintf("content must be at most %d characters", maxCommentContentLength))
	}
	ticket, err := s.tickets.GetTicketByID(ctx, ticketID, authorID)
	if err != nil {
		return nil, err
	}
	allowed, err := s.tickets.HasProjectPermission(ctx, ticket.ProjectID, authorID, project.PermissionCommentsCreate)
	if err != nil {
		return nil, apperror.New(apperror.ErrNotFound, "project not found or access denied")
	}
	if !allowed {
		return nil, apperror.New(apperror.ErrForbidden, "insufficient permissions: missing comments.create")
	}
	commentID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	comment := &Comment{
		ID:       commentID,
		APID:     activitypub.CommentAPID(s.apConfig, commentID),
		TicketID: ticketID,
		AuthorID: authorID,
		Content:  content,
	}
	result, err := s.repo.Create(ctx, comment)
	if err != nil {
		return nil, err
	}
	s.enqueueDeliveries(ctx, ticket.ProjectID, result.Deliveries)
	if s.notifications != nil {
		if err := s.notifications.NotifyCommentCreated(ctx, authorID, *ticket, content); err != nil {
			log.Printf("failed to create comment notifications for ticket %s: %v", ticket.ID, err)
		}
	}
	return comment, nil
}

// invalidCommentInput wraps a validation message with the comment input sentinel.
func invalidCommentInput(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidCommentInput, message)
}

// ListComments returns comments for a ticket visible to the current user.
func (s *Service) ListComments(ctx context.Context, ticketID, userID string, options CommentListOptions) ([]Comment, error) {
	if _, err := s.tickets.GetTicketByID(ctx, ticketID, userID); err != nil {
		return nil, err
	}
	options.Limit = normalizeCommentListLimit(options.Limit)
	options.Offset = normalizeCommentListOffset(options.Offset)
	return s.repo.ListByTicketID(ctx, ticketID, options)
}

// normalizeCommentListLimit bounds comment list sizes.
func normalizeCommentListLimit(limit int) int {
	if limit <= 0 {
		return defaultCommentListLimit
	}
	if limit > maxCommentListLimit {
		return maxCommentListLimit
	}
	return limit
}

// normalizeCommentListOffset clamps negative comment list offsets.
func normalizeCommentListOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

// DeleteComment removes a comment and emits a Delete activity.
func (s *Service) DeleteComment(ctx context.Context, commentID, userID string) error {
	comment, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		return apperror.New(apperror.ErrNotFound, "comment not found")
	}
	ticket, err := s.tickets.GetTicketByID(ctx, comment.TicketID, userID)
	if err != nil {
		return err
	}
	permission := project.PermissionCommentsModerate
	if comment.AuthorID == userID {
		permission = project.PermissionCommentsCreate
	}
	allowed, err := s.tickets.HasProjectPermission(ctx, ticket.ProjectID, userID, permission)
	if err != nil {
		return apperror.New(apperror.ErrNotFound, "project not found or access denied")
	}
	if !allowed {
		return apperror.New(apperror.ErrForbidden, "insufficient permissions: missing "+permission)
	}

	deleteResult, err := s.repo.Delete(ctx, commentID, userID)
	if err != nil {
		return err
	}
	s.enqueueDeliveries(ctx, deleteResult.ProjectID, deleteResult.Deliveries)
	return nil
}

// enqueueDeliveries queues delivery rows created in the comment transaction.
func (s *Service) enqueueDeliveries(ctx context.Context, projectID string, deliveries []apdelivery.QueueCandidate) {
	if s.delivery == nil || len(deliveries) == 0 {
		return
	}
	if err := s.delivery.EnqueuePersisted(ctx, deliveries); err != nil {
		log.Printf("failed to enqueue ActivityPub deliveries for project %s: %v", projectID, err)
	}
}
