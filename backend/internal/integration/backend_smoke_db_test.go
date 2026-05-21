//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/comment"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/stretchr/testify/require"
)

func TestBackendManagementSmokeFlow(t *testing.T) {
	db := openIntegrationDB(t)
	resetIntegrationDB(t, db)
	requireMigratedTable(t, db, "actors")
	requireMigratedTable(t, db, "activity_deliveries")

	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")

	userService := user.NewService(user.NewRepository(db, cfg), []byte("integration-secret"), cfg)
	projectService := project.NewService(project.NewRepository(db, cfg), cfg)
	ticketService := ticket.NewService(ticket.NewRepository(db, cfg), projectService, cfg)
	commentService := comment.NewService(comment.NewRepository(db, cfg), ticketService, cfg)
	deliveryService := delivery.NewService(delivery.NewRecipientRepository(db), delivery.NoopQueue{})
	projectService.SetDelivery(deliveryService)
	ticketService.SetDelivery(deliveryService)
	commentService.SetDelivery(deliveryService)

	owner, err := userService.RegisterUser(ctx, "smoke-owner", "smoke-owner@example.test", "password123")
	require.NoError(t, err)
	token, err := userService.Login(ctx, owner.Email, "password123")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	member, err := userService.RegisterUser(ctx, "smoke-member", "smoke-member@example.test", "password123")
	require.NoError(t, err)
	rejectedUser, err := userService.RegisterUser(ctx, "smoke-rejected", "smoke-rejected@example.test", "password123")
	require.NoError(t, err)

	createdProject, err := projectService.CreateProject(ctx, "Backend Smoke Board", "Full backend smoke flow", owner.ID)
	require.NoError(t, err)
	requireObjectType(t, db, createdProject.APID, "Group")
	requireProjectRole(t, db, owner.ID, createdProject.ID, project.RoleOwner)

	acceptedInvite, err := projectService.AddMemberToProject(ctx, createdProject.ID, owner.ID, member.ID, project.RoleDeveloper)
	require.NoError(t, err)
	requireActivityType(t, db, acceptedInvite.APID, "Invite")
	require.NoError(t, projectService.AcceptInvite(ctx, acceptedInvite.ID, member.ID))
	requireProjectRole(t, db, member.ID, createdProject.ID, project.RoleDeveloper)
	requireFollow(t, db, member.ID, createdProject.ID, "accepted")

	rejectedInvite, err := projectService.AddMemberToProject(ctx, createdProject.ID, owner.ID, rejectedUser.ID, project.RoleViewer)
	require.NoError(t, err)
	require.NoError(t, projectService.RejectInvite(ctx, rejectedInvite.ID, rejectedUser.ID))
	requireInviteStatus(t, db, rejectedInvite.ID, "rejected")
	requireNoProjectMember(t, db, rejectedUser.ID, createdProject.ID)

	remoteFollower := createRemoteActorWithPublicKey(t, ctx, db, "https://remote.example/users/smoke-follower", "smoke-follower", "public key")
	_, err = db.ExecContext(ctx, `
		INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
		VALUES ($1, $2, 'accepted')
	`, remoteFollower.ID, createdProject.ID)
	require.NoError(t, err)

	createdTicket, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
		Title:       "Smoke ticket",
		Description: "Created by the backend smoke flow.",
		Priority:    "high",
		Type:        "task",
	}, createdProject.ID, owner.ID)
	require.NoError(t, err)
	requireObjectType(t, db, createdTicket.APID, "Ticket")
	requireInboxItem(t, db, createdProject.ID, "Create", createdTicket.APID)
	requireOutboxItem(t, db, owner.ID, "Create", createdTicket.APID)
	requireDeliveryForObject(t, db, "Create", createdTicket.APID, remoteFollower.InboxURL)

	status := "done"
	assigneeID := member.ID
	assigneePatch := &assigneeID
	require.NoError(t, ticketService.UpdateTicket(ctx, ticket.UpdateTicketRequest{
		Status:     &status,
		AssigneeID: &assigneePatch,
	}, createdTicket.ID, member.ID))
	requireActivityForObjectAndActor(t, db, "Update", createdTicket.APID, member.ID)
	requireActivityForObjectAndTarget(t, db, "Add", member.APID, createdTicket.APID)
	requireDeliveryForObject(t, db, "Update", createdTicket.APID, remoteFollower.InboxURL)
	requireTicketResolved(t, db, createdTicket.ID)

	createdComment, err := commentService.CreateComment(ctx, createdTicket.ID, member.ID, "Smoke comment.")
	require.NoError(t, err)
	requireObjectType(t, db, createdComment.APID, "Note")
	requireInboxItem(t, db, createdProject.ID, "Create", createdComment.APID)
	requireOutboxItem(t, db, member.ID, "Create", createdComment.APID)
	requireDeliveryForObject(t, db, "Create", createdComment.APID, remoteFollower.InboxURL)

	require.NoError(t, commentService.DeleteComment(ctx, createdComment.ID, owner.ID))
	requireObjectDeleted(t, db, createdComment.APID)
	requireActivityForObjectAndActor(t, db, "Delete", createdComment.APID, owner.ID)
	requireDeliveryForObject(t, db, "Delete", createdComment.APID, remoteFollower.InboxURL)
}

func requireMigratedTable(t *testing.T, db interface {
	Get(dest any, query string, args ...any) error
}, tableName string) {
	t.Helper()

	var exists bool
	require.NoError(t, db.Get(&exists, `SELECT to_regclass($1) IS NOT NULL`, tableName))
	require.True(t, exists, "expected migrated table %s", tableName)
}
