package ticket

import (
	"context"

	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/stretchr/testify/mock"
)

// MockRepository is a mock implementation of Repository
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, ticket *Ticket) ([]string, error) {
	args := m.Called(ctx, ticket)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockRepository) ListByProjectID(ctx context.Context, projectID string, options TicketListOptions) ([]Ticket, error) {
	args := m.Called(ctx, projectID, options)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Ticket), args.Error(1)
}

func (m *MockRepository) GetByID(ctx context.Context, id string) (*Ticket, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Ticket), args.Error(1)
}

func (m *MockRepository) Update(ctx context.Context, ticket *Ticket, actorID string) ([]string, error) {
	args := m.Called(ctx, ticket, actorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockRepository) Delete(ctx context.Context, id string, actorID string) (*DeleteResult, error) {
	args := m.Called(ctx, id, actorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DeleteResult), args.Error(1)
}

func (m *MockRepository) CreateLink(ctx context.Context, link *TicketLink) error {
	args := m.Called(ctx, link)
	return args.Error(0)
}

func (m *MockRepository) GetLinkByID(ctx context.Context, linkID string) (*TicketLink, error) {
	args := m.Called(ctx, linkID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*TicketLink), args.Error(1)
}

func (m *MockRepository) DeleteLink(ctx context.Context, linkID string) error {
	args := m.Called(ctx, linkID)
	return args.Error(0)
}

func (m *MockRepository) GetLinksByProjectID(ctx context.Context, projectID string) ([]TicketLink, error) {
	args := m.Called(ctx, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]TicketLink), args.Error(1)
}

// MockProjectChecker
type MockProjectChecker struct {
	mock.Mock
}

func (m *MockProjectChecker) GetProjectByID(ctx context.Context, projectID, userID string) (*project.Project, error) {
	args := m.Called(ctx, projectID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*project.Project), args.Error(1)
}

func (m *MockProjectChecker) GetProjectRole(ctx context.Context, projectID, userID string) (string, error) {
	args := m.Called(ctx, projectID, userID)
	return args.String(0), args.Error(1)
}

func (m *MockProjectChecker) HasProjectPermission(ctx context.Context, projectID, userID, permission string) (bool, error) {
	args := m.Called(ctx, projectID, userID, permission)
	return args.Bool(0), args.Error(1)
}
