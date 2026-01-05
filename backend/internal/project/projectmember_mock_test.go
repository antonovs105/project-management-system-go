package project

import (
	"context"

	"github.com/antonovs105/project-management-system-go/internal/projectmember"
	"github.com/stretchr/testify/mock"
)

// MockMemberService is a mock implementation of MemberAdder interface
type MockMemberService struct {
	mock.Mock
}

func (m *MockMemberService) AddMember(ctx context.Context, userID, projectID int64, role string) (*projectmember.ProjectMember, error) {
	args := m.Called(ctx, userID, projectID, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*projectmember.ProjectMember), args.Error(1)
}

func (m *MockMemberService) GetUserRole(ctx context.Context, userID, projectID int64) (string, error) {
	args := m.Called(ctx, userID, projectID)
	return args.String(0), args.Error(1)
}
