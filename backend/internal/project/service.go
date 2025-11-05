package project

import (
	"context"
	"errors"
	"log"

	"github.com/antonovs105/project-management-system-go/internal/projectmember"
)

type MemberAdder interface {
	AddMember(ctx context.Context, userID, projectID int64, role string) (*projectmember.ProjectMember, error)
	GetUserRole(ctx context.Context, userID, projectID int64) (string, error)
}

type Service struct {
	repo                 *Repository
	projectMemberService MemberAdder
}

func NewService(repo *Repository, pmService MemberAdder) *Service {
	return &Service{
		repo:                 repo,
		projectMemberService: pmService,
	}
}

// CreateProject is business logic for creating project
func (s *Service) CreateProject(ctx context.Context, name, description string, userID int64) (*Project, error) {
	// TODO: Business logc here

	p := &Project{
		Name:        name,
		Description: description,
		OwnerID:     userID,
	}

	err := s.repo.Create(ctx, p)
	if err != nil {
		return nil, err
	}

	// after creating project add creator in table project_members as owner
	_, err = s.projectMemberService.AddMember(ctx, userID, p.ID, "owner")
	if err != nil {
		log.Printf("CRITICAL: project %d created, but failed to add owner role: %v", p.ID, err)
		return nil, errors.New("failed to finalize project creation")
	}

	return p, nil
}

func (s *Service) GetProjectByID(ctx context.Context, projectID, userID int64) (*Project, error) {
	project, err := s.repo.GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// Checking access
	role, err := s.projectMemberService.GetUserRole(ctx, userID, projectID)
	if err != nil {
		// Если `err` (особенно `sql.ErrNoRows`), значит пользователь не участник проекта.
		return nil, errors.New("project not found or access denied")
	}

	log.Printf("User %d has role '%s' in project %d", userID, role, projectID)

	return project, nil
}

// ListUserProjects returns projects list of user
// For now just calls repository
func (s *Service) ListUserProjects(ctx context.Context, userID int64) ([]Project, error) {
	// TODO: add logic for projects secondary roles
	projects, err := s.repo.ListByOwnerID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return projects, nil
}

// UpdateProjectRequest struct for providing data for update
type UpdateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// UpdateProject logic for updating project
func (s *Service) UpdateProject(ctx context.Context, projectID, userID int64, req UpdateProjectRequest) error {
	// find project, check accwss
	projectToUpdate, err := s.GetProjectByID(ctx, projectID, userID)
	if err != nil {
		return err
	}

	// update rows
	if req.Name != nil {
		projectToUpdate.Name = *req.Name
	}
	if req.Description != nil {
		projectToUpdate.Description = *req.Description
	}

	// save changes
	return s.repo.Update(ctx, projectToUpdate)
}

// DeleteProject do i really need to write what it does?
func (s *Service) DeleteProject(ctx context.Context, projectID, userID int64) error {
	// Check is project exists and is it belongs to user
	_, err := s.GetProjectByID(ctx, projectID, userID)
	if err != nil {
		return err
	}

	// deleting project
	return s.repo.Delete(ctx, projectID)
}

func (s *Service) AddMemberToProject(ctx context.Context, projectID, currentUserID, newUserID int64, role string) error {
	// Check priviliges
	currentUserRole, err := s.projectMemberService.GetUserRole(ctx, currentUserID, projectID)
	if err != nil {
		return errors.New("access denied: you are not a member of this project")
	}

	if currentUserRole != "owner" && currentUserRole != "manager" {
		return errors.New("insufficient permissions: only owners or managers can add new members")
	}

	// If good, call projectMemberService to ad new user (newUserID).
	_, err = s.projectMemberService.AddMember(ctx, newUserID, projectID, role)
	if err != nil {
		// TODO: add more clarity errors
		return err
	}

	return nil
}
