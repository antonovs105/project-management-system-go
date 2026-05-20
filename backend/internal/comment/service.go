package comment

import (
	"context"

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
}

func NewService(repo Repository, tickets TicketChecker, apConfig activitypub.Config) *Service {
	return &Service{repo: repo, tickets: tickets, apConfig: apConfig}
}

func (s *Service) CreateComment(ctx context.Context, ticketID, authorID, content string) (*Comment, error) {
	if _, err := s.tickets.GetTicketByID(ctx, ticketID, authorID); err != nil {
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
	if err := s.repo.Create(ctx, comment); err != nil {
		return nil, err
	}
	return comment, nil
}

func (s *Service) ListComments(ctx context.Context, ticketID, userID string) ([]Comment, error) {
	if _, err := s.tickets.GetTicketByID(ctx, ticketID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListByTicketID(ctx, ticketID)
}
