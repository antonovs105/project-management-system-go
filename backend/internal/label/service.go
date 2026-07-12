package label

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/antonovs105/project-management-system-go/internal/apperror"
	"github.com/antonovs105/project-management-system-go/internal/project"
)

// colorPattern accepts CSS six-digit hexadecimal colors.
var colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// ErrInvalidInput reports malformed label input.
var ErrInvalidInput = errors.New("invalid label input")

// ProjectChecker exposes project access and permission checks.
type ProjectChecker interface {
	GetProjectByID(ctx context.Context, projectID, userID string) (*project.Project, error)
	HasProjectPermission(ctx context.Context, projectID, userID, permission string) (bool, error)
}

// Service enforces label access and validation.
type Service struct {
	repo     Repository
	projects ProjectChecker
}

// NewService creates a label service.
func NewService(repo Repository, projects ProjectChecker) *Service {
	return &Service{repo: repo, projects: projects}
}

// List returns labels visible to a project member.
func (s *Service) List(ctx context.Context, projectID, userID string) ([]Label, error) {
	if _, err := s.projects.GetProjectByID(ctx, projectID, userID); err != nil {
		return nil, apperror.New(apperror.ErrNotFound, "project not found or access denied")
	}
	return s.repo.List(ctx, projectID)
}

// Create validates and creates a project label.
func (s *Service) Create(ctx context.Context, projectID, userID, name, color string) (*Label, error) {
	if err := s.requireManage(ctx, projectID, userID); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	color = strings.ToUpper(strings.TrimSpace(color))
	if name == "" || utf8.RuneCountInString(name) > 50 || !colorPattern.MatchString(color) {
		return nil, ErrInvalidInput
	}
	item := &Label{ProjectID: projectID, Name: name, Color: color}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

// Delete removes a project label and all ticket assignments.
func (s *Service) Delete(ctx context.Context, projectID, labelID, userID string) error {
	if err := s.requireManage(ctx, projectID, userID); err != nil {
		return err
	}
	deleted, err := s.repo.Delete(ctx, projectID, labelID)
	if err != nil {
		return err
	}
	if !deleted {
		return apperror.New(apperror.ErrNotFound, "label not found")
	}
	return nil
}

// requireManage restricts label mutation to users who can update the project.
func (s *Service) requireManage(ctx context.Context, projectID, userID string) error {
	allowed, err := s.projects.HasProjectPermission(ctx, projectID, userID, project.PermissionProjectUpdate)
	if err != nil {
		return apperror.New(apperror.ErrNotFound, "project not found or access denied")
	}
	if !allowed {
		return apperror.New(apperror.ErrForbidden, "insufficient permissions: missing project.update")
	}
	return nil
}
