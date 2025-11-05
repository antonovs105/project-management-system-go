package projectmember

import "context"

type Service struct {
	repo *Repository
	// TODO: add dependencies from UserService/ProjectService for checkups
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// AddMember adds user into project
func (s *Service) AddMember(ctx context.Context, userID, projectID int64, role string) (*ProjectMember, error) {
	// TODO: check is userID exists
	// TODO: check is project exists

	pm := &ProjectMember{
		UserID:    userID,
		ProjectID: projectID,
		Role:      role,
	}

	err := s.repo.Add(ctx, pm)
	if err != nil {
		// TODO: handle error "already exists"
		return nil, err
	}
	return pm, nil
}

func (s *Service) GetUserRole(ctx context.Context, userID, projectID int64) (string, error) {
	return s.repo.GetUserRoleInProject(ctx, userID, projectID)
}
