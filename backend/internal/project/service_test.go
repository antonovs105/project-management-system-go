package project

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestService_CreateProject(t *testing.T) {
	mockRepo := new(MockRepository)
	mockPM := new(MockMemberService)
	service := NewService(mockRepo, mockPM, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

	ctx := context.Background()
	name := "Test Project"
	desc := "Description"
	userID := "user-1"

	t.Run("Success", func(t *testing.T) {
		// Expect Create to be called
		mockRepo.On("Create", ctx, mock.MatchedBy(func(p *Project) bool {
			return p.ID != "" && p.APID != "" && p.Name == name && p.OwnerID == userID
		})).Return(nil).Run(func(args mock.Arguments) {
			p := args.Get(1).(*Project)
			p.ID = "project-1"
		}).Once()

		p, err := service.CreateProject(ctx, name, desc, userID)

		assert.NoError(t, err)
		assert.NotNil(t, p)
		assert.Equal(t, "project-1", p.ID)
		mockRepo.AssertExpectations(t)
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
	service := NewService(mockRepo, mockPM, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

	ctx := context.Background()
	projectID := "project-1"
	userID := "user-1"

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
