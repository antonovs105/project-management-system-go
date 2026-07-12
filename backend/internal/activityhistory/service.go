package activityhistory

import "context"

// projectReadPermission is the stable permission required to inspect history.
const projectReadPermission = "project.read"

// Service authorizes user-facing project history.
type Service struct {
	repository  EventRepository
	archives    ArchiveRepository
	permissions PermissionChecker
}

// NewService returns an activity-history service.
func NewService(repository EventRepository, permissions PermissionChecker, archives ...ArchiveRepository) *Service {
	service := &Service{repository: repository, permissions: permissions}
	if value, ok := repository.(ArchiveRepository); ok {
		service.archives = value
	}
	if len(archives) > 0 {
		service.archives = archives[0]
	}
	return service
}

// List validates access and pagination before loading events.
func (s *Service) List(ctx context.Context, projectID, userID string, limit, offset int) ([]Event, error) {
	allowed, err := s.permissions.HasPermission(ctx, projectID, userID, projectReadPermission)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return s.repository.List(ctx, projectID, limit, offset)
}

// ListArchivedProjects returns the current user's restorable projects.
func (s *Service) ListArchivedProjects(ctx context.Context, userID string) ([]ArchivedProject, error) {
	return s.archives.ListArchivedProjects(ctx, userID)
}

// ListArchivedTickets authorizes and returns restorable project tickets.
func (s *Service) ListArchivedTickets(ctx context.Context, projectID, userID string) ([]ArchivedTicket, error) {
	if err := s.require(ctx, projectID, userID, "project.read"); err != nil {
		return nil, err
	}
	return s.archives.ListArchivedTickets(ctx, projectID)
}

// SetProjectArchived authorizes an optimistic project archive transition.
func (s *Service) SetProjectArchived(ctx context.Context, projectID, userID string, version int64, archived bool) error {
	if err := s.require(ctx, projectID, userID, "project.update"); err != nil {
		return err
	}
	return s.archives.SetProjectArchived(ctx, projectID, userID, version, archived)
}

// SetTicketArchived authorizes an optimistic ticket archive transition.
func (s *Service) SetTicketArchived(ctx context.Context, ticketID, userID string, version int64, archived bool) error {
	projectID, err := s.archives.TicketProjectID(ctx, ticketID)
	if err != nil {
		return err
	}
	if err := s.require(ctx, projectID, userID, "tickets.update"); err != nil {
		return err
	}
	return s.archives.SetTicketArchived(ctx, ticketID, userID, version, archived)
}

// require checks one project permission.
func (s *Service) require(ctx context.Context, projectID, userID, permission string) error {
	allowed, err := s.permissions.HasPermission(ctx, projectID, userID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}
