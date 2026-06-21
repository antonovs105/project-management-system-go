package ticket

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type testEventPublisher struct {
	events []Event
}

type testDeliveryEnqueuer struct {
	deliveries []apdelivery.QueueCandidate
}

func (d *testDeliveryEnqueuer) EnqueuePersisted(ctx context.Context, deliveries []apdelivery.QueueCandidate) error {
	d.deliveries = append(d.deliveries, deliveries...)
	return nil
}

func (p *testEventPublisher) PublishTicketEvent(event Event) {
	p.events = append(p.events, event)
}

type testNotificationSink struct {
	assignments []Ticket
	assignees   []string
	actors      []string
}

func (s *testNotificationSink) NotifyTicketAssigned(_ context.Context, assigneeID, actorID string, ticket Ticket) error {
	s.assignees = append(s.assignees, assigneeID)
	s.actors = append(s.actors, actorID)
	s.assignments = append(s.assignments, ticket)
	return nil
}

func TestService_CreateTicket(t *testing.T) {
	mockRepo := new(MockRepository)
	mockProject := new(MockProjectChecker)
	service := NewService(mockRepo, mockProject, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

	ctx := context.Background()
	projectID := "project-1"
	reporterID := "user-1"
	req := CreateTicketRequest{
		Title:       "New Ticket",
		Description: "Desc",
		Priority:    "high",
		Type:        "task",
	}

	t.Run("Success", func(t *testing.T) {
		delivery := &testDeliveryEnqueuer{}
		service.SetDelivery(delivery)
		mockProject.On("HasProjectPermission", ctx, projectID, reporterID, project.PermissionTicketsCreate).Return(true, nil).Once()
		mockRepo.On("Create", ctx, mock.AnythingOfType("*ticket.Ticket")).Return(&ActivityResult{
			ActivityIDs: []string{"activity-1"},
			Deliveries:  []apdelivery.QueueCandidate{{ID: "delivery-1", MaxAttempts: 10}},
		}, nil).Run(func(args mock.Arguments) {
			ticket := args.Get(1).(*Ticket)
			ticket.ID = "ticket-1"
		}).Once()

		ticket, err := service.CreateTicket(ctx, req, projectID, reporterID)

		assert.NoError(t, err)
		assert.NotNil(t, ticket)
		assert.Equal(t, "task", ticket.Type)
		assert.Equal(t, []apdelivery.QueueCandidate{{ID: "delivery-1", MaxAttempts: 10}}, delivery.deliveries)
		mockRepo.AssertExpectations(t)
		mockProject.AssertExpectations(t)
	})

	t.Run("InvalidType", func(t *testing.T) {
		mockProject.On("HasProjectPermission", ctx, projectID, reporterID, project.PermissionTicketsCreate).Return(true, nil).Once()
		invalidReq := req
		invalidReq.Type = "invalid"

		ticket, err := service.CreateTicket(ctx, invalidReq, projectID, reporterID)

		assert.Error(t, err)
		assert.Nil(t, ticket)
		assert.ErrorIs(t, err, ErrInvalidTicketInput)
		assert.Contains(t, err.Error(), "invalid ticket type")
	})

	t.Run("InvalidTitle", func(t *testing.T) {
		mockProject.On("HasProjectPermission", ctx, projectID, reporterID, project.PermissionTicketsCreate).Return(true, nil).Once()
		invalidReq := req
		invalidReq.Title = "   "

		ticket, err := service.CreateTicket(ctx, invalidReq, projectID, reporterID)

		assert.ErrorIs(t, err, ErrInvalidTicketInput)
		assert.Nil(t, ticket)
		assert.Contains(t, err.Error(), "title is required")
	})

	t.Run("InvalidPriority", func(t *testing.T) {
		mockProject.On("HasProjectPermission", ctx, projectID, reporterID, project.PermissionTicketsCreate).Return(true, nil).Once()
		invalidReq := req
		invalidReq.Priority = "eventually"

		ticket, err := service.CreateTicket(ctx, invalidReq, projectID, reporterID)

		assert.ErrorIs(t, err, ErrInvalidTicketInput)
		assert.Nil(t, ticket)
		assert.Contains(t, err.Error(), "invalid ticket priority")
	})

	t.Run("RejectsOversizedTitle", func(t *testing.T) {
		mockProject.On("HasProjectPermission", ctx, projectID, reporterID, project.PermissionTicketsCreate).Return(true, nil).Once()
		invalidReq := req
		invalidReq.Title = strings.Repeat("x", maxTicketTitleLength+1)

		ticket, err := service.CreateTicket(ctx, invalidReq, projectID, reporterID)

		assert.ErrorIs(t, err, ErrInvalidTicketInput)
		assert.Nil(t, ticket)
		assert.Contains(t, err.Error(), "title must be at most")
	})

	t.Run("RejectsOversizedDescription", func(t *testing.T) {
		mockProject.On("HasProjectPermission", ctx, projectID, reporterID, project.PermissionTicketsCreate).Return(true, nil).Once()
		invalidReq := req
		invalidReq.Description = strings.Repeat("x", maxTicketDescriptionLength+1)

		ticket, err := service.CreateTicket(ctx, invalidReq, projectID, reporterID)

		assert.ErrorIs(t, err, ErrInvalidTicketInput)
		assert.Nil(t, ticket)
		assert.Contains(t, err.Error(), "description must be at most")
	})
}

func TestService_GetTicketByID(t *testing.T) {
	mockRepo := new(MockRepository)
	mockProject := new(MockProjectChecker)
	service := NewService(mockRepo, mockProject, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

	ctx := context.Background()
	ticketID := "ticket-1"
	projectID := "project-1"
	userID := "user-1"

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

func TestService_UpdateTicketValidatesFields(t *testing.T) {
	mockRepo := new(MockRepository)
	mockProject := new(MockProjectChecker)
	service := NewService(mockRepo, mockProject, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

	ctx := context.Background()
	projectID := "project-1"
	ticketID := "ticket-1"
	actorID := "member-1"
	blankTitle := " "

	storedTicket := &Ticket{
		ID:          ticketID,
		ProjectID:   projectID,
		ReporterID:  "reporter-1",
		Title:       "Ticket",
		Description: "Description",
		Status:      "open",
		Priority:    "medium",
		Type:        "task",
	}

	mockRepo.On("GetByID", ctx, ticketID).Return(storedTicket, nil).Once()
	mockProject.On("GetProjectByID", ctx, projectID, actorID).Return(&project.Project{ID: projectID}, nil).Once()
	mockProject.On("HasProjectPermission", ctx, projectID, actorID, project.PermissionTicketsUpdate).Return(true, nil).Once()

	err := service.UpdateTicket(ctx, UpdateTicketRequest{Title: &blankTitle}, ticketID, actorID)

	assert.ErrorIs(t, err, ErrInvalidTicketInput)
	assert.Contains(t, err.Error(), "title is required")
	mockRepo.AssertNotCalled(t, "Update")
}

func TestService_UpdateTicketValidatesMetadataLength(t *testing.T) {
	ctx := context.Background()
	projectID := "project-1"
	ticketID := "ticket-1"
	actorID := "member-1"

	t.Run("RejectsOversizedTitle", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockProject := new(MockProjectChecker)
		service := NewService(mockRepo, mockProject, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
		longTitle := strings.Repeat("x", maxTicketTitleLength+1)

		mockRepo.On("GetByID", ctx, ticketID).Return(&Ticket{
			ID:          ticketID,
			ProjectID:   projectID,
			Title:       "Ticket",
			Description: "Description",
			Status:      "open",
			Priority:    "medium",
			Type:        "task",
		}, nil).Once()
		mockProject.On("GetProjectByID", ctx, projectID, actorID).Return(&project.Project{ID: projectID}, nil).Once()
		mockProject.On("HasProjectPermission", ctx, projectID, actorID, project.PermissionTicketsUpdate).Return(true, nil).Once()

		err := service.UpdateTicket(ctx, UpdateTicketRequest{Title: &longTitle}, ticketID, actorID)

		assert.ErrorIs(t, err, ErrInvalidTicketInput)
		assert.Contains(t, err.Error(), "title must be at most")
		mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything)
		mockRepo.AssertExpectations(t)
		mockProject.AssertExpectations(t)
	})

	t.Run("RejectsOversizedDescription", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockProject := new(MockProjectChecker)
		service := NewService(mockRepo, mockProject, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
		longDescription := strings.Repeat("x", maxTicketDescriptionLength+1)

		mockRepo.On("GetByID", ctx, ticketID).Return(&Ticket{
			ID:          ticketID,
			ProjectID:   projectID,
			Title:       "Ticket",
			Description: "Description",
			Status:      "open",
			Priority:    "medium",
			Type:        "task",
		}, nil).Once()
		mockProject.On("GetProjectByID", ctx, projectID, actorID).Return(&project.Project{ID: projectID}, nil).Once()
		mockProject.On("HasProjectPermission", ctx, projectID, actorID, project.PermissionTicketsUpdate).Return(true, nil).Once()

		err := service.UpdateTicket(ctx, UpdateTicketRequest{Description: &longDescription}, ticketID, actorID)

		assert.ErrorIs(t, err, ErrInvalidTicketInput)
		assert.Contains(t, err.Error(), "description must be at most")
		mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything)
		mockRepo.AssertExpectations(t)
		mockProject.AssertExpectations(t)
	})
}

func TestService_UpdateTicketUsesActingUser(t *testing.T) {
	mockRepo := new(MockRepository)
	mockProject := new(MockProjectChecker)
	service := NewService(mockRepo, mockProject, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	events := &testEventPublisher{}
	service.SetEventPublisher(events)

	ctx := context.Background()
	projectID := "project-1"
	ticketID := "ticket-1"
	reporterID := "reporter-1"
	actorID := "member-1"
	status := "done"

	storedTicket := &Ticket{
		ID:          ticketID,
		ProjectID:   projectID,
		ReporterID:  reporterID,
		Title:       "Ticket",
		Description: "Description",
		Status:      "open",
		Priority:    "medium",
		Type:        "task",
	}

	mockRepo.On("GetByID", ctx, ticketID).Return(storedTicket, nil).Once()
	mockProject.On("GetProjectByID", ctx, projectID, actorID).Return(&project.Project{ID: projectID}, nil).Once()
	mockProject.On("HasProjectPermission", ctx, projectID, actorID, project.PermissionTicketsUpdate).Return(true, nil).Once()
	mockRepo.On("Update", ctx, mock.MatchedBy(func(t *Ticket) bool {
		return t.ID == ticketID && t.Status == status && t.ReporterID == reporterID
	}), actorID).Return(&ActivityResult{ActivityIDs: []string{"activity-1"}}, nil).Once()

	err := service.UpdateTicket(ctx, UpdateTicketRequest{Status: &status}, ticketID, actorID)

	assert.NoError(t, err)
	require.Len(t, events.events, 1)
	assert.Equal(t, EventTicketUpdated, events.events[0].Type)
	assert.Equal(t, projectID, events.events[0].ProjectID)
	assert.Equal(t, ticketID, events.events[0].TicketID)
	mockRepo.AssertExpectations(t)
	mockProject.AssertExpectations(t)
}

func TestService_UpdateTicketNotifiesNewAssignee(t *testing.T) {
	mockRepo := new(MockRepository)
	mockProject := new(MockProjectChecker)
	service := NewService(mockRepo, mockProject, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	notifications := &testNotificationSink{}
	service.SetNotificationSink(notifications)

	ctx := context.Background()
	projectID := "project-1"
	ticketID := "ticket-1"
	actorID := "member-1"
	assigneeID := "member-2"
	assigneePtr := &assigneeID

	storedTicket := &Ticket{
		ID:          ticketID,
		ProjectID:   projectID,
		ReporterID:  "reporter-1",
		Title:       "Ticket",
		Description: "Description",
		Status:      "open",
		Priority:    "medium",
		Type:        "task",
	}

	mockRepo.On("GetByID", ctx, ticketID).Return(storedTicket, nil).Once()
	mockProject.On("GetProjectByID", ctx, projectID, actorID).Return(&project.Project{ID: projectID}, nil).Once()
	mockProject.On("HasProjectPermission", ctx, projectID, actorID, project.PermissionTicketsUpdate).Return(true, nil).Once()
	mockRepo.On("Update", ctx, mock.MatchedBy(func(t *Ticket) bool {
		return t.ID == ticketID && t.AssigneeID != nil && *t.AssigneeID == assigneeID
	}), actorID).Return(&ActivityResult{ActivityIDs: []string{"activity-1"}}, nil).Once()

	err := service.UpdateTicket(ctx, UpdateTicketRequest{AssigneeID: &assigneePtr}, ticketID, actorID)

	require.NoError(t, err)
	require.Len(t, notifications.assignments, 1)
	assert.Equal(t, assigneeID, notifications.assignees[0])
	assert.Equal(t, actorID, notifications.actors[0])
	assert.Equal(t, ticketID, notifications.assignments[0].ID)
	mockRepo.AssertExpectations(t)
	mockProject.AssertExpectations(t)
}

func TestService_MoveTicket(t *testing.T) {
	mockRepo := new(MockRepository)
	mockProject := new(MockProjectChecker)
	service := NewService(mockRepo, mockProject, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	events := &testEventPublisher{}
	service.SetEventPublisher(events)

	ctx := context.Background()
	projectID := "project-1"
	ticketID := "ticket-1"
	actorID := "member-1"
	status := "review"
	beforeID := "ticket-2"

	storedTicket := &Ticket{
		ID:          ticketID,
		ProjectID:   projectID,
		ReporterID:  "reporter-1",
		Title:       "Ticket",
		Description: "Description",
		Status:      "open",
		Priority:    "medium",
		Type:        "task",
	}
	movedTicket := *storedTicket
	movedTicket.Status = status

	mockRepo.On("GetByID", ctx, ticketID).Return(storedTicket, nil).Once()
	mockProject.On("GetProjectByID", ctx, projectID, actorID).Return(&project.Project{ID: projectID}, nil).Once()
	mockProject.On("HasProjectPermission", ctx, projectID, actorID, project.PermissionTicketsUpdate).Return(true, nil).Once()
	mockRepo.On("Move", ctx, ticketID, actorID, status, &beforeID, (*string)(nil)).Return(&movedTicket, &ActivityResult{ActivityIDs: []string{"activity-1"}}, nil).Once()

	moved, err := service.MoveTicket(ctx, MoveTicketRequest{Status: status, BeforeTicketID: &beforeID}, ticketID, actorID)

	require.NoError(t, err)
	require.NotNil(t, moved)
	assert.Equal(t, status, moved.Status)
	require.Len(t, events.events, 1)
	assert.Equal(t, EventTicketUpdated, events.events[0].Type)
	mockRepo.AssertExpectations(t)
	mockProject.AssertExpectations(t)
}

func TestService_AddTicketLink(t *testing.T) {
	mockRepo := new(MockRepository)
	mockProject := new(MockProjectChecker)
	service := NewService(mockRepo, mockProject, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

	ctx := context.Background()
	projectID := "project-1"
	userID := "user-1"
	sourceID := "ticket-100"
	targetID := "ticket-101"

	sourceTicket := &Ticket{ID: sourceID, ProjectID: projectID}
	targetTicket := &Ticket{ID: targetID, ProjectID: projectID}

	t.Run("Success", func(t *testing.T) {
		// Mock GetTicketByID for source and target
		// Since GetTicketByID calls calls GetByID then GetProjectByID
		mockRepo.On("GetByID", ctx, sourceID).Return(sourceTicket, nil).Once()
		mockProject.On("GetProjectByID", ctx, projectID, userID).Return(&project.Project{}, nil).Once()

		mockRepo.On("GetByID", ctx, targetID).Return(targetTicket, nil).Once()
		mockProject.On("GetProjectByID", ctx, projectID, userID).Return(&project.Project{}, nil).Once()
		mockProject.On("HasProjectPermission", ctx, projectID, userID, project.PermissionTicketsUpdate).Return(true, nil).Once()

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

		tktA := &Ticket{ID: "ticket-100", ProjectID: projectID}
		tktB := &Ticket{ID: "ticket-101", ProjectID: projectID}

		mockRepo.On("GetByID", ctx, "ticket-101").Return(tktB, nil).Once()
		mockProject.On("GetProjectByID", ctx, projectID, userID).Return(&project.Project{}, nil).Once()

		mockRepo.On("GetByID", ctx, "ticket-100").Return(tktA, nil).Once()
		mockProject.On("GetProjectByID", ctx, projectID, userID).Return(&project.Project{}, nil).Once()
		mockProject.On("HasProjectPermission", ctx, projectID, userID, project.PermissionTicketsUpdate).Return(true, nil).Once()

		// Existing links: A->B
		existingLink := TicketLink{SourceID: "ticket-100", TargetID: "ticket-101", LinkType: "blocks"}
		mockRepo.On("GetLinksByProjectID", ctx, projectID).Return([]TicketLink{existingLink}, nil).Once()

		err := service.AddTicketLink(ctx, "ticket-101", "ticket-100", "blocks", projectID, userID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cycle detected")
	})
}

func TestService_GetTicketGraph(t *testing.T) {
	mockRepo := new(MockRepository)
	mockProject := new(MockProjectChecker)
	service := NewService(mockRepo, mockProject, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

	ctx := context.Background()
	projectID := "project-1"
	userID := "user-1"

	t.Run("Success", func(t *testing.T) {
		mockProject.On("GetProjectByID", ctx, projectID, userID).Return(&project.Project{}, nil).Once()

		tickets := []Ticket{
			{ID: "ticket-1", Title: "Epic", Type: "epic", CreatedAt: time.Now()},
			{ID: "ticket-2", Title: "Task", Type: "task", ParentID: stringPtr("ticket-1"), CreatedAt: time.Now()},
		}
		mockRepo.On("ListByProjectID", ctx, projectID, TicketListOptions{Limit: graphNodeLimit + 1}).Return(tickets, nil).Once()
		mockRepo.On("GetLinksByProjectID", ctx, projectID).Return([]TicketLink{}, nil).Once()

		graph, err := service.GetTicketGraph(ctx, projectID, userID)

		assert.NoError(t, err)
		assert.NotNil(t, graph)
		assert.Len(t, graph.Nodes, 2)
		assert.Len(t, graph.Links, 1) // 1 hierarchy link
		assert.Equal(t, "hierarchy", graph.Links[0].Type)
		assert.Equal(t, graphNodeLimit, graph.Limit)
		assert.False(t, graph.Truncated)
	})

	t.Run("MarksTruncatedGraphAndSkipsDanglingLinks", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockProject := new(MockProjectChecker)
		service := NewService(mockRepo, mockProject, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
		mockProject.On("GetProjectByID", ctx, projectID, userID).Return(&project.Project{}, nil).Once()

		tickets := make([]Ticket, 0, graphNodeLimit+1)
		for i := 0; i < graphNodeLimit+1; i++ {
			tickets = append(tickets, Ticket{
				ID:        fmt.Sprintf("ticket-%03d", i),
				Title:     fmt.Sprintf("Ticket %d", i),
				Type:      "task",
				CreatedAt: time.Now(),
			})
		}
		tickets[1].ParentID = stringPtr("ticket-000")
		links := []TicketLink{
			{SourceID: "ticket-000", TargetID: "ticket-001", LinkType: "relates_to"},
			{SourceID: "ticket-000", TargetID: "ticket-500", LinkType: "blocks"},
		}
		mockRepo.On("ListByProjectID", ctx, projectID, TicketListOptions{Limit: graphNodeLimit + 1}).Return(tickets, nil).Once()
		mockRepo.On("GetLinksByProjectID", ctx, projectID).Return(links, nil).Once()

		graph, err := service.GetTicketGraph(ctx, projectID, userID)

		require.NoError(t, err)
		require.NotNil(t, graph)
		assert.True(t, graph.Truncated)
		assert.Equal(t, graphNodeLimit, graph.Limit)
		assert.Len(t, graph.Nodes, graphNodeLimit)
		for _, link := range graph.Links {
			assert.NotEqual(t, "ticket-500", link.Source)
			assert.NotEqual(t, "ticket-500", link.Target)
		}
		assert.Len(t, graph.Links, 2)
		mockRepo.AssertExpectations(t)
		mockProject.AssertExpectations(t)
	})
}

func stringPtr(s string) *string { return &s }
