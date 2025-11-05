package project

import (
	"context"
	"errors"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
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

	// TODO: after creating project need to add creator in table project_members as owner

	return p, nil
}

func (s *Service) GetProjectByID(ctx context.Context, projectID, userID int64) (*Project, error) {
	project, err := s.repo.GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// Checking access
	// for now sim[le]
	if project.OwnerID != userID {
		return nil, errors.New("project not found or access denied")
	}

	return project, nil
}
