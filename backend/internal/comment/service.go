package comment

import (
	"context"
	"log"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
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
	EnqueueProjectFollowers(ctx context.Context, projectID string, activityIDs ...string) error
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
	s.enqueueProjectFollowers(ctx, ticket.ProjectID, activityID)
	return comment, nil
}

func (s *Service) ListComments(ctx context.Context, ticketID, userID string) ([]Comment, error) {
	if _, err := s.tickets.GetTicketByID(ctx, ticketID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListByTicketID(ctx, ticketID)
}

func (s *Service) enqueueProjectFollowers(ctx context.Context, projectID string, activityIDs ...string) {
	if s.delivery == nil || len(activityIDs) == 0 {
		return
	}
	if err := s.delivery.EnqueueProjectFollowers(ctx, projectID, activityIDs...); err != nil {
		log.Printf("failed to enqueue ActivityPub deliveries for project %s: %v", projectID, err)
	}
}
