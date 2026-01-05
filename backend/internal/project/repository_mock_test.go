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

func (m *MockRepository) GetByID(ctx context.Context, id int64) (*Project, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Project), args.Error(1)
}

func (m *MockRepository) ListByOwnerID(ctx context.Context, ownerID int64) ([]Project, error) {
	args := m.Called(ctx, ownerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Project), args.Error(1)
}

func (m *MockRepository) Update(ctx context.Context, project *Project) error {
	args := m.Called(ctx, project)
	return args.Error(0)
}

func (m *MockRepository) Delete(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
