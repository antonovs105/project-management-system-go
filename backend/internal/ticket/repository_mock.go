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

func (m *MockRepository) Create(ctx context.Context, ticket *Ticket) error {
	args := m.Called(ctx, ticket)
	return args.Error(0)
}

func (m *MockRepository) ListByProjectID(ctx context.Context, projectID int64) ([]Ticket, error) {
	args := m.Called(ctx, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Ticket), args.Error(1)
}

func (m *MockRepository) GetByID(ctx context.Context, id int64) (*Ticket, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Ticket), args.Error(1)
}

func (m *MockRepository) Update(ctx context.Context, ticket *Ticket) error {
	args := m.Called(ctx, ticket)
	return args.Error(0)
}

func (m *MockRepository) Delete(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockRepository) CreateLink(ctx context.Context, link *TicketLink) error {
	args := m.Called(ctx, link)
	return args.Error(0)
}

func (m *MockRepository) DeleteLink(ctx context.Context, linkID int64) error {
	args := m.Called(ctx, linkID)
	return args.Error(0)
}

func (m *MockRepository) GetLinksByProjectID(ctx context.Context, projectID int64) ([]TicketLink, error) {
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

func (m *MockProjectChecker) GetProjectByID(ctx context.Context, projectID, userID int64) (*project.Project, error) {
	args := m.Called(ctx, projectID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*project.Project), args.Error(1)
}
