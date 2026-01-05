package ticket

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestService_CreateTicket(t *testing.T) {
	mockRepo := new(MockRepository)
	mockProject := new(MockProjectChecker)
	service := NewService(mockRepo, mockProject)

	ctx := context.Background()
	projectID := int64(10)
	reporterID := int64(1)
	req := CreateTicketRequest{
		Title:       "New Ticket",
		Description: "Desc",
		Priority:    "high",
		Type:        "task",
	}

	t.Run("Success", func(t *testing.T) {
		mockProject.On("GetProjectByID", ctx, projectID, reporterID).Return(&project.Project{ID: projectID}, nil).Once()
		mockRepo.On("Create", ctx, mock.AnythingOfType("*ticket.Ticket")).Return(nil).Run(func(args mock.Arguments) {
			ticket := args.Get(1).(*Ticket)
			ticket.ID = 100
		}).Once()

		ticket, err := service.CreateTicket(ctx, req, projectID, reporterID)

		assert.NoError(t, err)
		assert.NotNil(t, ticket)
		assert.Equal(t, "task", ticket.Type)
		mockRepo.AssertExpectations(t)
		mockProject.AssertExpectations(t)
	})

	t.Run("InvalidType", func(t *testing.T) {
		mockProject.On("GetProjectByID", ctx, projectID, reporterID).Return(&project.Project{ID: projectID}, nil).Once()
		invalidReq := req
		invalidReq.Type = "invalid"

		ticket, err := service.CreateTicket(ctx, invalidReq, projectID, reporterID)

		assert.Error(t, err)
		assert.Nil(t, ticket)
		assert.Equal(t, "invalid ticket type", err.Error())
	})
}

func TestService_GetTicketByID(t *testing.T) {
	mockRepo := new(MockRepository)
	mockProject := new(MockProjectChecker)
	service := NewService(mockRepo, mockProject)

	ctx := context.Background()
	ticketID := int64(100)
	projectID := int64(10)
	userID := int64(1)

	expectedTicket := &Ticket{
		ID:        ticketID,
		ProjectID: projectID,
		Title:     "Ticket",
	}

	t.Run("Success", func(t *testing.T) {
		mockRepo.On("GetByID", ctx, ticketID).Return(expectedTicket, nil).Once()
		mockProject.On("GetProjectByID", ctx, projectID, userID).Return(&project.Project{ID: projectID}, nil).Once()

		ticket, err := service.GetTicketByID(ctx, ticketID, userID)

		assert.NoError(t, err)
		assert.Equal(t, expectedTicket, ticket)
	})

	t.Run("TicketNotFound", func(t *testing.T) {
		mockRepo.On("GetByID", ctx, ticketID).Return(nil, errors.New("not found")).Once()

		ticket, err := service.GetTicketByID(ctx, ticketID, userID)

		assert.Error(t, err)
		assert.Nil(t, ticket)
		assert.Equal(t, "ticket not found", err.Error())
	})
}

func TestService_AddTicketLink(t *testing.T) {
	mockRepo := new(MockRepository)
	mockProject := new(MockProjectChecker)
	service := NewService(mockRepo, mockProject)

	ctx := context.Background()
	projectID := int64(10)
	userID := int64(1)
	sourceID := int64(100)
	targetID := int64(101)

	sourceTicket := &Ticket{ID: sourceID, ProjectID: projectID}
	targetTicket := &Ticket{ID: targetID, ProjectID: projectID}

	t.Run("Success", func(t *testing.T) {
		// Mock GetTicketByID for source and target
		// Since GetTicketByID calls calls GetByID then GetProjectByID
		mockRepo.On("GetByID", ctx, sourceID).Return(sourceTicket, nil).Once()
		mockProject.On("GetProjectByID", ctx, projectID, userID).Return(&project.Project{}, nil).Once()

		mockRepo.On("GetByID", ctx, targetID).Return(targetTicket, nil).Once()
		mockProject.On("GetProjectByID", ctx, projectID, userID).Return(&project.Project{}, nil).Once()

		// Mock GetLinksByProjectID for cycle check (empty list = no cycle)
		mockRepo.On("GetLinksByProjectID", ctx, projectID).Return([]TicketLink{}, nil).Once()

		// Mock CreateLink
		mockRepo.On("CreateLink", ctx, mock.AnythingOfType("*ticket.TicketLink")).Return(nil).Once()

		err := service.AddTicketLink(ctx, sourceID, targetID, "blocks", projectID, userID)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("CycleDetected", func(t *testing.T) {
		// A -> B. Try to add B -> A.
		// Source: B (101), Target: A (100)

		tktA := &Ticket{ID: 100, ProjectID: projectID}
		tktB := &Ticket{ID: 101, ProjectID: projectID}

		mockRepo.On("GetByID", ctx, int64(101)).Return(tktB, nil).Once()
		mockProject.On("GetProjectByID", ctx, projectID, userID).Return(&project.Project{}, nil).Once()

		mockRepo.On("GetByID", ctx, int64(100)).Return(tktA, nil).Once()
		mockProject.On("GetProjectByID", ctx, projectID, userID).Return(&project.Project{}, nil).Once()

		// Existing links: A->B
		existingLink := TicketLink{SourceID: 100, TargetID: 101, LinkType: "blocks"}
		mockRepo.On("GetLinksByProjectID", ctx, projectID).Return([]TicketLink{existingLink}, nil).Once()

		err := service.AddTicketLink(ctx, 101, 100, "blocks", projectID, userID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cycle detected")
	})
}

func TestService_GetTicketGraph(t *testing.T) {
	mockRepo := new(MockRepository)
	mockProject := new(MockProjectChecker)
	service := NewService(mockRepo, mockProject)

	ctx := context.Background()
	projectID := int64(10)
	userID := int64(1)

	t.Run("Success", func(t *testing.T) {
		mockProject.On("GetProjectByID", ctx, projectID, userID).Return(&project.Project{}, nil).Once()

		tickets := []Ticket{
			{ID: 1, Title: "Epic", Type: "epic", CreatedAt: time.Now()},
			{ID: 2, Title: "Task", Type: "task", ParentID: int64Ptr(1), CreatedAt: time.Now()},
		}
		mockRepo.On("ListByProjectID", ctx, projectID).Return(tickets, nil).Once()
		mockRepo.On("GetLinksByProjectID", ctx, projectID).Return([]TicketLink{}, nil).Once()

		graph, err := service.GetTicketGraph(ctx, projectID, userID)

		assert.NoError(t, err)
		assert.NotNil(t, graph)
		assert.Len(t, graph.Nodes, 2)
		assert.Len(t, graph.Links, 1) // 1 hierarchy link
		assert.Equal(t, "hierarchy", graph.Links[0].Type)
	})
}

func int64Ptr(i int64) *int64 { return &i }
