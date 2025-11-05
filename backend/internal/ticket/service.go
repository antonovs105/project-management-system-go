package ticket

import (
	"context"
	"errors"

	"github.com/antonovs105/project-management-system-go/internal/project"
)

// ProjectChecker interface
type ProjectChecker interface {
	GetProjectByID(ctx context.Context, projectID, userID int64) (*project.Project, error)
}

type Service struct {
	repo           *Repository
	projectService ProjectChecker
}

func NewService(repo *Repository, projectService ProjectChecker) *Service {
	return &Service{
		repo:           repo,
		projectService: projectService,
	}
}

// CreateTicketRequest DTO for ticket creation
type CreateTicketRequest struct {
	Title       string
	Description string
	Priority    string
	AssigneeID  *int64
}

// CreateTicket logic for ticket creation
func (s *Service) CreateTicket(ctx context.Context, req CreateTicketRequest, projectID, reporterID int64) (*Ticket, error) {
	// checking access
	_, err := s.projectService.GetProjectByID(ctx, projectID, reporterID)
	if err != nil {
		return nil, err
	}

	// TODO: check is AssigneeID a project member

	t := &Ticket{
		Title:       req.Title,
		Description: req.Description,
		Status:      "new",
		Priority:    req.Priority,
		ProjectID:   projectID,
		ReporterID:  reporterID,
		AssigneeID:  req.AssigneeID,
	}

	err = s.repo.Create(ctx, t)
	if err != nil {
		return nil, err
	}

	return t, nil
}

// ListTicketsInProject logic for ticket list
func (s *Service) ListTicketsInProject(ctx context.Context, projectID, userID int64) ([]Ticket, error) {
	// check access
	_, err := s.projectService.GetProjectByID(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}

	return s.repo.ListByProjectID(ctx, projectID)
}

// GetTicketByID gogic to get single ticket
func (s *Service) GetTicketByID(ctx context.Context, ticketID, userID int64) (*Ticket, error) {
	ticket, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, errors.New("ticket not found")
	}

	// check access
	_, err = s.projectService.GetProjectByID(ctx, ticket.ProjectID, userID)
	if err != nil {
		return nil, errors.New("ticket not found or access denied")
	}

	return ticket, nil
}

// UpdateTicketRequest DTO for updating ticket
type UpdateTicketRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	Priority    *string `json:"priority"`
	AssigneeID  **int64 `json:"assignee_id"`
}

// UpdateTicket logic for update
func (s *Service) UpdateTicket(ctx context.Context, req UpdateTicketRequest, ticketID, userID int64) error {
	// find ticket, check access
	ticketToUpdate, err := s.GetTicketByID(ctx, ticketID, userID)
	if err != nil {
		return err
	}

	// TODO: add more advanced check

	// update rows
	if req.Title != nil {
		ticketToUpdate.Title = *req.Title
	}
	if req.Description != nil {
		ticketToUpdate.Description = *req.Description
	}
	if req.Status != nil {
		ticketToUpdate.Status = *req.Status
	}
	if req.Priority != nil {
		ticketToUpdate.Priority = *req.Priority
	}
	// logic for AssigneeID
	if req.AssigneeID != nil {
		ticketToUpdate.AssigneeID = *req.AssigneeID
	}

	return s.repo.Update(ctx, ticketToUpdate)
}

// DeleteTicket logic for deleting
func (s *Service) DeleteTicket(ctx context.Context, ticketID, userID int64) error {
	// check access
	_, err := s.GetTicketByID(ctx, ticketID, userID)
	if err != nil {
		return err
	}

	return s.repo.Delete(ctx, ticketID)
}
