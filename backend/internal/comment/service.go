package comment

import (
	"context"
	"errors"
	"log"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
)

type TicketChecker interface {
	GetTicketByID(ctx context.Context, ticketID, userID string) (*ticket.Ticket, error)
}

type Service struct {
	repo     Repository
	tickets  TicketChecker
	apConfig activitypub.Config
	delivery DeliveryEnqueuer
}

func NewService(repo Repository, tickets TicketChecker, apConfig activitypub.Config) *Service {
	return &Service{repo: repo, tickets: tickets, apConfig: apConfig}
}

type DeliveryEnqueuer interface {
	Enqueue(ctx context.Context, activityID string, targetInboxURL string) (*apdelivery.Delivery, error)
	EnqueueProjectFollowers(ctx context.Context, projectID string, activityIDs ...string) error
	EnqueueProjectTicketRecipients(ctx context.Context, projectID string, ticketID string, activityIDs ...string) error
}

func (s *Service) SetDelivery(delivery DeliveryEnqueuer) {
	s.delivery = delivery
}

func (s *Service) CreateComment(ctx context.Context, ticketID, authorID, content string) (*Comment, error) {
	ticket, err := s.tickets.GetTicketByID(ctx, ticketID, authorID)
	if err != nil {
		return nil, err
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

func (s *Service) ListComments(ctx context.Context, ticketID, userID string) ([]Comment, error) {
	if _, err := s.tickets.GetTicketByID(ctx, ticketID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListByTicketID(ctx, ticketID)
}

func (s *Service) DeleteComment(ctx context.Context, commentID, userID string) error {
	comment, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		return errors.New("comment not found")
	}
	if _, err := s.tickets.GetTicketByID(ctx, comment.TicketID, userID); err != nil {
		return err
	}

	deleteResult, err := s.repo.Delete(ctx, commentID, userID)
	if err != nil {
		return err
	}
	s.enqueueRecipientInboxes(ctx, deleteResult)
	return nil
}

func (s *Service) enqueueProjectFollowers(ctx context.Context, projectID string, activityIDs ...string) {
	if s.delivery == nil || len(activityIDs) == 0 {
		return
	}
	if err := s.delivery.EnqueueProjectFollowers(ctx, projectID, activityIDs...); err != nil {
		log.Printf("failed to enqueue ActivityPub deliveries for project %s: %v", projectID, err)
	}
}

func (s *Service) enqueueProjectTicketRecipients(ctx context.Context, projectID, ticketID string, activityIDs ...string) {
	if s.delivery == nil || len(activityIDs) == 0 {
		return
	}
	if err := s.delivery.EnqueueProjectTicketRecipients(ctx, projectID, ticketID, activityIDs...); err != nil {
		log.Printf("failed to enqueue ActivityPub deliveries for project %s ticket %s: %v", projectID, ticketID, err)
	}
}

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
