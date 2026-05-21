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

func (m *MockRepository) ListByOwnerID(ctx context.Context, ownerID string, options ProjectListOptions) ([]Project, error) {
	args := m.Called(ctx, ownerID, options)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Project), args.Error(1)
}

func (m *MockRepository) GetMemberRole(ctx context.Context, userID, projectID string) (string, error) {
	args := m.Called(ctx, userID, projectID)
	return args.String(0), args.Error(1)
}

func (m *MockRepository) HasPermission(ctx context.Context, projectID, userID, permission string) (bool, error) {
	args := m.Called(ctx, projectID, userID, permission)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) CountMembersWithPermission(ctx context.Context, projectID, permission string) (int, error) {
	args := m.Called(ctx, projectID, permission)
	return args.Int(0), args.Error(1)
}

func (m *MockRepository) RoleHasPermission(ctx context.Context, roleID, permission string) (bool, error) {
	args := m.Called(ctx, roleID, permission)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ResolveRole(ctx context.Context, projectID, roleRef string) (*ProjectRole, error) {
	args := m.Called(ctx, projectID, roleRef)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ProjectRole), args.Error(1)
}

func (m *MockRepository) ListRoles(ctx context.Context, projectID string) ([]ProjectRole, error) {
	args := m.Called(ctx, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]ProjectRole), args.Error(1)
}

func (m *MockRepository) GetRoleByID(ctx context.Context, projectID, roleID string) (*ProjectRole, error) {
	args := m.Called(ctx, projectID, roleID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ProjectRole), args.Error(1)
}

func (m *MockRepository) CreateRole(ctx context.Context, role *ProjectRole) error {
	args := m.Called(ctx, role)
	return args.Error(0)
}

func (m *MockRepository) UpdateRole(ctx context.Context, role *ProjectRole) error {
	args := m.Called(ctx, role)
	return args.Error(0)
}

func (m *MockRepository) DeleteRole(ctx context.Context, projectID, roleID string) error {
	args := m.Called(ctx, projectID, roleID)
	return args.Error(0)
}

func (m *MockRepository) RoleAssignmentCount(ctx context.Context, projectID, roleID string) (int, error) {
	args := m.Called(ctx, projectID, roleID)
	return args.Int(0), args.Error(1)
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

func (m *MockRepository) RemoveMember(ctx context.Context, projectID, actorID, targetUserID string) (*MembershipResult, error) {
	args := m.Called(ctx, projectID, actorID, targetUserID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*MembershipResult), args.Error(1)
}

func (m *MockRepository) CreateInvite(ctx context.Context, invite *ProjectInvite) (*MembershipResult, error) {
	args := m.Called(ctx, invite)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*MembershipResult), args.Error(1)
}

func (m *MockRepository) GetInviteByID(ctx context.Context, inviteID string) (*ProjectInvite, error) {
	args := m.Called(ctx, inviteID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ProjectInvite), args.Error(1)
}

func (m *MockRepository) AcceptInvite(ctx context.Context, inviteID, userID string) (*MembershipResult, error) {
	args := m.Called(ctx, inviteID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*MembershipResult), args.Error(1)
}

func (m *MockRepository) RejectInvite(ctx context.Context, inviteID, userID string) (*MembershipResult, error) {
	args := m.Called(ctx, inviteID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*MembershipResult), args.Error(1)
}

func (m *MockRepository) RevokeInvite(ctx context.Context, inviteID, userID string) (*MembershipResult, error) {
	args := m.Called(ctx, inviteID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*MembershipResult), args.Error(1)
}
