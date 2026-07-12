//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/account"
	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/comment"
	"github.com/antonovs105/project-management-system-go/internal/notification"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNotificationPreferencesAndEmailOutboxAgainstPostgreSQL(t *testing.T) {
	source := os.Getenv("TEST_DB_SOURCE")
	if source == "" {
		t.Skip("set TEST_DB_SOURCE to run integration tests")
	}
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	db := newSchemaIntegrationDB(t, ctx, source, "test_notifications_"+suffix)
	config := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	userService := user.NewService(user.NewRepository(db, config), []byte("integration-secret"), config)
	actor, err := userService.RegisterUser(ctx, "notify_actor_"+suffix, "notify_actor_"+suffix+"@example.test", "password123")
	require.NoError(t, err)
	recipient, err := userService.RegisterUser(ctx, "notify_recipient_"+suffix, "notify_recipient_"+suffix+"@example.test", "password123")
	require.NoError(t, err)
	projectService := project.NewService(project.NewRepository(db, config), config)
	createdProject, err := projectService.CreateProject(ctx, "Notification "+suffix, "delivery test", actor.ID)
	require.NoError(t, err)
	ticketService := ticket.NewService(ticket.NewRepository(db, config), projectService, config)
	createdTicket, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
		Title: "Notification evidence", Priority: "medium", Type: "task",
	}, createdProject.ID, actor.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE users SET email_verified = true WHERE id = $1`, recipient.ID)
	require.NoError(t, err)

	service := notification.NewService(
		notification.NewRepository(db),
		notification.WithEmailQueue(account.NewRepository(db), "https://progo.example.test"),
	)
	require.NoError(t, service.NotifyTicketAssigned(ctx, recipient.ID, actor.ID, *createdTicket))
	values, err := service.List(ctx, recipient.ID, notification.ListOptions{})
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.Equal(t, notification.TypeTicketAssigned, values[0].Type)
	var queued int
	require.NoError(t, db.GetContext(ctx, &queued, `SELECT count(*) FROM email_outbox WHERE recipient = $1`, recipient.Email))
	require.Equal(t, 1, queued)

	preference, err := service.UpdatePreference(ctx, recipient.ID, notification.Preference{
		Type: notification.TypeTicketAssigned, InAppEnabled: false, EmailEnabled: false,
	})
	require.NoError(t, err)
	require.False(t, preference.InAppEnabled)
	require.NoError(t, service.NotifyTicketAssigned(ctx, recipient.ID, actor.ID, *createdTicket))
	values, err = service.List(ctx, recipient.ID, notification.ListOptions{})
	require.NoError(t, err)
	require.Len(t, values, 1)

	_, err = db.ExecContext(ctx, `UPDATE tickets SET due_date = $1 WHERE id = $2`, time.Now().UTC().Add(time.Hour), createdTicket.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO ticket_assignees (ticket_id, actor_id) VALUES ($1, $2)`, createdTicket.ID, recipient.ID)
	require.NoError(t, err)
	processed, err := service.DispatchDueNotifications(ctx, time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	processed, err = service.DispatchDueNotifications(ctx, time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	var dueCount int
	require.NoError(t, db.GetContext(ctx, &dueCount, `SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'ticket.due_soon'`, recipient.ID))
	require.Equal(t, 1, dueCount)
	require.NoError(t, db.GetContext(ctx, &queued, `SELECT count(*) FROM email_outbox WHERE recipient = $1`, recipient.Email))
	require.Equal(t, 2, queued)

	projectService.SetNotificationSink(service)
	ticketService.SetNotificationSink(service)
	invite, err := projectService.AddMemberToProject(ctx, createdProject.ID, actor.ID, recipient.ID, project.RoleDeveloper)
	require.NoError(t, err)
	require.NoError(t, projectService.AcceptInvite(ctx, invite.ID, recipient.ID))
	assignedID := recipient.ID
	workflowTicket, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
		Title: "Workflow notification", Priority: "medium", Type: "task", AssigneeID: &assignedID,
	}, createdProject.ID, actor.ID)
	require.NoError(t, err)
	status := "in_progress"
	require.NoError(t, ticketService.UpdateTicket(ctx, ticket.UpdateTicketRequest{Status: &status}, workflowTicket.ID, actor.ID))
	commentService := comment.NewService(comment.NewRepository(db, config), ticketService, config)
	commentService.SetNotificationSink(service)
	_, err = commentService.CreateComment(ctx, workflowTicket.ID, actor.ID, "Please review this update")
	require.NoError(t, err)
	_, err = commentService.CreateComment(ctx, workflowTicket.ID, actor.ID, "@"+recipient.Username+" please check the evidence")
	require.NoError(t, err)
	_, err = projectService.UpdateProjectMemberRole(ctx, createdProject.ID, actor.ID, recipient.ID, project.RoleViewer)
	require.NoError(t, err)
	require.NoError(t, service.NotifySecurityEvent(ctx, recipient.ID, "Security check", "A sensitive account action occurred."))

	var types []string
	require.NoError(t, db.SelectContext(ctx, &types, `SELECT type FROM notifications WHERE user_id = $1 ORDER BY created_at`, recipient.ID))
	require.Contains(t, types, notification.TypeProjectInvited)
	require.Contains(t, types, notification.TypeTicketStatusChanged)
	require.Contains(t, types, notification.TypeCommentCreated)
	require.Contains(t, types, notification.TypeCommentMentioned)
	require.Contains(t, types, notification.TypeProjectRoleChanged)
	require.Contains(t, types, notification.TypeSecurityEvent)
}
