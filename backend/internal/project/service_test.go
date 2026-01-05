package project

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestService_CreateProject(t *testing.T) {
	mockRepo := new(MockRepository)
	mockPM := new(MockMemberService)
	service := NewService(mockRepo, mockPM)

	ctx := context.Background()
	name := "Test Project"
	desc := "Description"
	userID := int64(1)

	t.Run("Success", func(t *testing.T) {
		// Expect Create to be called
		mockRepo.On("Create", ctx, mock.MatchedBy(func(p *Project) bool {
			return p.Name == name && p.OwnerID == userID
		})).Return(nil).Run(func(args mock.Arguments) {
			p := args.Get(1).(*Project)
			p.ID = 100 // Simulate ID assignment
		}).Once()

		// Expect AddMember to be called
		mockPM.On("AddMember", ctx, userID, int64(100), "owner").Return(nil, nil).Once()

		p, err := service.CreateProject(ctx, name, desc, userID)

		assert.NoError(t, err)
		assert.NotNil(t, p)
		assert.Equal(t, int64(100), p.ID)
		mockRepo.AssertExpectations(t)
		mockPM.AssertExpectations(t)
	})

	t.Run("RepoError", func(t *testing.T) {
		mockRepo.On("Create", ctx, mock.Anything).Return(errors.New("db error")).Once()

		p, err := service.CreateProject(ctx, name, desc, userID)

		assert.Error(t, err)
		assert.Nil(t, p)
		mockRepo.AssertExpectations(t)
	})
}

func TestService_GetProjectByID(t *testing.T) {
	mockRepo := new(MockRepository)
	mockPM := new(MockMemberService)
	service := NewService(mockRepo, mockPM)

	ctx := context.Background()
	projectID := int64(100)
	userID := int64(1)

	expectedProject := &Project{
		ID:        projectID,
		Name:      "Test Project",
		OwnerID:   userID,
		CreatedAt: time.Now(),
	}

	t.Run("Success", func(t *testing.T) {
		mockRepo.On("GetByID", ctx, projectID).Return(expectedProject, nil).Once()
		mockPM.On("GetUserRole", ctx, userID, projectID).Return("owner", nil).Once()

		p, err := service.GetProjectByID(ctx, projectID, userID)

		assert.NoError(t, err)
		assert.Equal(t, expectedProject, p)
		mockRepo.AssertExpectations(t)
		mockPM.AssertExpectations(t)
	})

	t.Run("AccessDenied", func(t *testing.T) {
		mockRepo.On("GetByID", ctx, projectID).Return(expectedProject, nil).Once()
		mockPM.On("GetUserRole", ctx, userID, projectID).Return("", errors.New("not a member")).Once()

		p, err := service.GetProjectByID(ctx, projectID, userID)

		assert.Error(t, err)
		assert.Nil(t, p)
		assert.Contains(t, err.Error(), "project not found or access denied")
		mockRepo.AssertExpectations(t)
	})
}
