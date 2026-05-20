package project

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// MockRepository is a mock implementation of Repository
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, project *Project) error {
	args := m.Called(ctx, project)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id string) (*Project, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Project), args.Error(1)
}

func (m *MockRepository) ListByOwnerID(ctx context.Context, ownerID string) ([]Project, error) {
	args := m.Called(ctx, ownerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Project), args.Error(1)
}

func (m *MockRepository) GetUserRole(ctx context.Context, userID, projectID string) (string, error) {
	args := m.Called(ctx, userID, projectID)
	return args.String(0), args.Error(1)
}

func (m *MockRepository) IsProjectMember(ctx context.Context, projectID, userID string) (bool, error) {
	args := m.Called(ctx, projectID, userID)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) HasPendingInvite(ctx context.Context, projectID, userID string) (bool, error) {
	args := m.Called(ctx, projectID, userID)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) Update(ctx context.Context, project *Project, actorID string) (*UpdateResult, error) {
	args := m.Called(ctx, project, actorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*UpdateResult), args.Error(1)
}

func (m *MockRepository) Delete(ctx context.Context, id string, actorID string) (*DeleteResult, error) {
	args := m.Called(ctx, id, actorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DeleteResult), args.Error(1)
}

func (m *MockRepository) RemoveMember(ctx context.Context, projectID, actorID, targetUserID string) error {
	args := m.Called(ctx, projectID, actorID, targetUserID)
	return args.Error(0)
}

func (m *MockRepository) CreateInvite(ctx context.Context, invite *ProjectInvite) error {
	args := m.Called(ctx, invite)
	return args.Error(0)
}

func (m *MockRepository) GetInviteByID(ctx context.Context, inviteID string) (*ProjectInvite, error) {
	args := m.Called(ctx, inviteID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ProjectInvite), args.Error(1)
}

func (m *MockRepository) AcceptInvite(ctx context.Context, inviteID, userID string) error {
	args := m.Called(ctx, inviteID, userID)
	return args.Error(0)
}

func (m *MockRepository) RejectInvite(ctx context.Context, inviteID, userID string) error {
	args := m.Called(ctx, inviteID, userID)
	return args.Error(0)
}

func (m *MockRepository) RevokeInvite(ctx context.Context, inviteID, userID string) error {
	args := m.Called(ctx, inviteID, userID)
	return args.Error(0)
}
