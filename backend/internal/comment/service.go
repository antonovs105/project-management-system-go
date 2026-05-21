package comment

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
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
)

// TicketChecker exposes ticket lookups and project roles needed by comments.
type TicketChecker interface {
	GetTicketByID(ctx context.Context, ticketID, userID string) (*ticket.Ticket, error)
	HasProjectPermission(ctx context.Context, projectID, userID, permission string) (bool, error)
}

// Service contains comment workflows and ActivityPub side effects.
type Service struct {
	repo     Repository
	tickets  TicketChecker
	apConfig activitypub.Config
	delivery DeliveryEnqueuer
}

// NewService creates a comment service.
func NewService(repo Repository, tickets TicketChecker, apConfig activitypub.Config) *Service {
	return &Service{repo: repo, tickets: tickets, apConfig: apConfig}
}

// DeliveryEnqueuer queues federation deliveries created by comment actions.
type DeliveryEnqueuer interface {
	Enqueue(ctx context.Context, activityID string, targetInboxURL string) (*apdelivery.Delivery, error)
	EnqueueProjectFollowers(ctx context.Context, projectID string, activityIDs ...string) error
	EnqueueProjectTicketRecipients(ctx context.Context, projectID string, ticketID string, activityIDs ...string) error
}

// SetDelivery attaches the delivery queue used for comment federation.
func (s *Service) SetDelivery(delivery DeliveryEnqueuer) {
	s.delivery = delivery
}

// CreateComment creates a Note on a ticket and records its Create activity.
func (s *Service) CreateComment(ctx context.Context, ticketID, authorID, content string) (*Comment, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, invalidCommentInput("content is required")
	}
	ticket, err := s.tickets.GetTicketByID(ctx, ticketID, authorID)
	if err != nil {
		return nil, err
	}
	allowed, err := s.tickets.HasProjectPermission(ctx, ticket.ProjectID, authorID, project.PermissionCommentsCreate)
	if err != nil {
		return nil, errors.New("project not found or access denied")
	}
	if !allowed {
		return nil, errors.New("insufficient permissions: missing comments.create")
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
	activityID, err := s.repo.Create(ctx, comment)
	if err != nil {
		return nil, err
	}
	s.enqueueProjectTicketRecipients(ctx, ticket.ProjectID, ticket.ID, activityID)
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
		return errors.New("comment not found")
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
		return errors.New("project not found or access denied")
	}
	if !allowed {
		return errors.New("insufficient permissions: missing " + permission)
	}

	deleteResult, err := s.repo.Delete(ctx, commentID, userID)
	if err != nil {
		return err
	}
	s.enqueueRecipientInboxes(ctx, deleteResult)
	return nil
}

// enqueueProjectFollowers queues comment activities to all remote project followers.
func (s *Service) enqueueProjectFollowers(ctx context.Context, projectID string, activityIDs ...string) {
	if s.delivery == nil || len(activityIDs) == 0 {
		return
	}
	if err := s.delivery.EnqueueProjectFollowers(ctx, projectID, activityIDs...); err != nil {
		log.Printf("failed to enqueue ActivityPub deliveries for project %s: %v", projectID, err)
	}
}

// enqueueProjectTicketRecipients queues comment activities to ticket-related recipients.
func (s *Service) enqueueProjectTicketRecipients(ctx context.Context, projectID, ticketID string, activityIDs ...string) {
	if s.delivery == nil || len(activityIDs) == 0 {
		return
	}
	if err := s.delivery.EnqueueProjectTicketRecipients(ctx, projectID, ticketID, activityIDs...); err != nil {
		log.Printf("failed to enqueue ActivityPub deliveries for project %s ticket %s: %v", projectID, ticketID, err)
	}
}

// enqueueRecipientInboxes queues delete activities to precomputed remote inboxes.
func (s *Service) enqueueRecipientInboxes(ctx context.Context, result *DeleteResult) {
	if s.delivery == nil || result == nil || result.ActivityID == "" {
		return
	}
	for _, inbox := range result.RecipientInboxes {
		if inbox == "" {
			continue
		}
		if _, err := s.delivery.Enqueue(ctx, result.ActivityID, inbox); err != nil {
			log.Printf("failed to enqueue ActivityPub delivery for project %s inbox %s: %v", result.ProjectID, inbox, err)
		}
	}
}
