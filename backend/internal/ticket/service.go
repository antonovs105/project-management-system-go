package ticket

import (
	"context"

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
