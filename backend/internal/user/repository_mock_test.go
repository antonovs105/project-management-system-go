package user

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"
)

// MockRepository is a mock implementation of Repository
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) CreateUser(ctx context.Context, user *User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockRepository) CreateUserWithOAuthIdentity(ctx context.Context, user *User, identity *OAuthIdentity) error {
	args := m.Called(ctx, user, identity)
	return args.Error(0)
}

func (m *MockRepository) CreateAdminIfNoAdmin(ctx context.Context, user *User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockRepository) GetOAuthIdentity(ctx context.Context, provider, subject string) (*OAuthIdentity, error) {
	args := m.Called(ctx, provider, subject)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*OAuthIdentity), args.Error(1)
}

func (m *MockRepository) UpdateOAuthIdentity(ctx context.Context, identity *OAuthIdentity) error {
	args := m.Called(ctx, identity)
	return args.Error(0)
}

func (m *MockRepository) CreateOAuthLoginCode(ctx context.Context, userID, codeHash string, expiresAt time.Time) error {
	args := m.Called(ctx, userID, codeHash, expiresAt)
	return args.Error(0)
}

func (m *MockRepository) ConsumeOAuthLoginCode(ctx context.Context, codeHash string, now time.Time) (*User, error) {
	args := m.Called(ctx, codeHash, now)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockRepository) GetUserByID(ctx context.Context, userID string) (*User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockRepository) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	args := m.Called(ctx, userID, passwordHash)
	return args.Error(0)
}

func (m *MockRepository) TokenVersion(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func (m *MockRepository) InstanceRole(ctx context.Context, userID string) (string, error) {
	args := m.Called(ctx, userID)
	return args.String(0), args.Error(1)
}

func (m *MockRepository) ListUsers(ctx context.Context, options ListUsersOptions) ([]User, error) {
	args := m.Called(ctx, options)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]User), args.Error(1)
}

func (m *MockRepository) UpdateInstanceRole(ctx context.Context, adminUserID, userID, role string) (*User, error) {
	args := m.Called(ctx, adminUserID, userID, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}
