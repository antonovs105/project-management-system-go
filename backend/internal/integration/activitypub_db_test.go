//go:build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/c2s"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	apmoderation "github.com/antonovs105/project-management-system-go/internal/activitypub/moderation"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteinbox"
	"github.com/antonovs105/project-management-system-go/internal/comment"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivityPubFoundationFlow(t *testing.T) {
	db := openIntegrationDB(t)
	resetIntegrationDB(t, db)

	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")

	userRepo := user.NewRepository(db, cfg)
	userService := user.NewService(userRepo, []byte("integration-secret"), cfg)

	projectRepo := project.NewRepository(db, cfg)
	projectService := project.NewService(projectRepo, cfg)

	ticketRepo := ticket.NewRepository(db, cfg)
	ticketService := ticket.NewService(ticketRepo, projectService, cfg)

	commentRepo := comment.NewRepository(db, cfg)
	commentService := comment.NewService(commentRepo, ticketService, cfg)

	deliveryService := delivery.NewService(delivery.NewRecipientRepository(db), delivery.NoopQueue{})
	projectService.SetDelivery(deliveryService)
	ticketService.SetDelivery(deliveryService)
	commentService.SetDelivery(deliveryService)

	owner, err := userService.RegisterUser(ctx, "owner", "owner@example.test", "password123")
	require.NoError(t, err)
	member, err := userService.RegisterUser(ctx, "member", "member@example.test", "password123")
	require.NoError(t, err)

	requireRowCount(t, db, "actors", 2)
	requireRowCount(t, db, "actor_keys", 2)
	requireObjectType(t, db, owner.APID, "Person")

	project, err := projectService.CreateProject(ctx, "Federated Board", "A local ActivityPub board", owner.ID)
	require.NoError(t, err)
	requireObjectType(t, db, project.APID, "Group")
	requireProjectRole(t, db, owner.ID, project.ID, "owner")
	requireFollow(t, db, owner.ID, project.ID, "accepted")

	invite, err := projectService.AddMemberToProject(ctx, project.ID, owner.ID, member.ID, "developer")
	require.NoError(t, err)
	requireActivityType(t, db, invite.APID, "Invite")

	require.NoError(t, projectService.AcceptInvite(ctx, invite.ID, member.ID))
	requireProjectRole(t, db, member.ID, project.ID, "developer")
	requireFollow(t, db, member.ID, project.ID, "accepted")

	inviteGuardActivityCount := activityCount(t, db)
	_, err = projectService.AddMemberToProject(ctx, project.ID, owner.ID, member.ID, "viewer")
	require.Error(t, err)
	require.Contains(t, err.Error(), "already a project member")
	requireActivityCount(t, db, inviteGuardActivityCount)

	rejectUser, err := userService.RegisterUser(ctx, "reject-user", "reject-user@example.test", "password123")
	require.NoError(t, err)
	rejectInvite, err := projectService.AddMemberToProject(ctx, project.ID, owner.ID, rejectUser.ID, "viewer")
	require.NoError(t, err)
	requireActivityType(t, db, rejectInvite.APID, "Invite")

	rejectInvalidActivityCount := activityCount(t, db)
	err = projectService.RejectInvite(ctx, rejectInvite.ID, owner.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invite does not belong")
	requireActivityCount(t, db, rejectInvalidActivityCount)

	require.NoError(t, projectService.RejectInvite(ctx, rejectInvite.ID, rejectUser.ID))
	requireInviteStatus(t, db, rejectInvite.ID, "rejected")
	requireActivityForObjectAndActor(t, db, "Reject", rejectInvite.APID, rejectUser.ID)
	requireNoProjectMember(t, db, rejectUser.ID, project.ID)

	rejectedActivityCount := activityCount(t, db)
	err = projectService.AcceptInvite(ctx, rejectInvite.ID, rejectUser.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invite is not pending")
	requireActivityCount(t, db, rejectedActivityCount)

	pendingUser, err := userService.RegisterUser(ctx, "pending-user", "pending-user@example.test", "password123")
	require.NoError(t, err)
	pendingInvite, err := projectService.AddMemberToProject(ctx, project.ID, owner.ID, pendingUser.ID, "developer")
	require.NoError(t, err)
	pendingActivityCount := activityCount(t, db)
	_, err = projectService.AddMemberToProject(ctx, project.ID, owner.ID, pendingUser.ID, "developer")
	require.Error(t, err)
	require.Contains(t, err.Error(), "pending invite already exists")
	requireActivityCount(t, db, pendingActivityCount)
	requireInviteStatus(t, db, pendingInvite.ID, "pending")

	revokeUser, err := userService.RegisterUser(ctx, "revoke-user", "revoke-user@example.test", "password123")
	require.NoError(t, err)
	revokeInvite, err := projectService.AddMemberToProject(ctx, project.ID, owner.ID, revokeUser.ID, "viewer")
	require.NoError(t, err)

	revokeInvalidActivityCount := activityCount(t, db)
	err = projectService.RevokeInvite(ctx, revokeInvite.ID, member.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "only owners or managers can revoke invites")
	requireActivityCount(t, db, revokeInvalidActivityCount)

	require.NoError(t, projectService.RevokeInvite(ctx, revokeInvite.ID, owner.ID))
	requireInviteStatus(t, db, revokeInvite.ID, "revoked")
	requireActivityForObjectAndActor(t, db, "Undo", revokeInvite.APID, owner.ID)
	requireNoProjectMember(t, db, revokeUser.ID, project.ID)

	revokedActivityCount := activityCount(t, db)
	err = projectService.AcceptInvite(ctx, revokeInvite.ID, revokeUser.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invite is not pending")
	requireActivityCount(t, db, revokedActivityCount)

	remoteInbox := "https://remote.example/users/follower/inbox"
	remoteFollower := &remoteactor.Actor{
		APID:              "https://remote.example/users/follower",
		Type:              "Person",
		PreferredUsername: "follower",
		Handle:            "follower@remote.example",
		Name:              "Remote Follower",
		InboxURL:          remoteInbox,
		OutboxURL:         "https://remote.example/users/follower/outbox",
		PublicKeyID:       "https://remote.example/users/follower#main-key",
		PublicKeyPEM:      "public key",
		Document:          []byte(`{"id":"https://remote.example/users/follower","type":"Person"}`),
	}
	require.NoError(t, remoteactor.NewRepository(db).UpsertRemoteActor(ctx, remoteFollower))
	_, err = db.ExecContext(ctx, `
		INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
		VALUES ($1, $2, 'accepted')
	`, remoteFollower.ID, project.ID)
	require.NoError(t, err)

	remoteInvitee := createRemoteActorWithPublicKey(t, ctx, db, "https://remote.example/users/invitee", "invitee", "public key")
	remoteInvite, err := projectService.AddMemberToProject(ctx, project.ID, owner.ID, remoteInvitee.ID, "viewer")
	require.NoError(t, err)
	requireActivityType(t, db, remoteInvite.APID, "Invite")
	requireDeliveryForObject(t, db, "Invite", project.APID, remoteInvitee.InboxURL)
	require.NoError(t, projectService.RevokeInvite(ctx, remoteInvite.ID, owner.ID))
	requireInviteStatus(t, db, remoteInvite.ID, "revoked")
	requireActivityForObjectAndActor(t, db, "Undo", remoteInvite.APID, owner.ID)
	requireDeliveryForObject(t, db, "Undo", remoteInvite.APID, remoteInvitee.InboxURL)

	createdTicket, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
		Title:       "Design local outbox",
		Description: "Persist local AP activities",
		Priority:    "high",
		Type:        "task",
	}, project.ID, owner.ID)
	require.NoError(t, err)
	requireObjectType(t, db, createdTicket.APID, "Ticket")
	requireActivityForObject(t, db, "Create", createdTicket.APID)
	ticketDoc := requireActivityPubDocument(t, db, cfg, "/tickets/"+createdTicket.ID)
	require.Equal(t, "forge:Ticket", ticketDoc["type"])
	createTicketActivityAPID := requireActivityAPIDForObject(t, db, "Create", createdTicket.APID)
	createTicketActivityDoc := requireActivityPubDocument(t, db, cfg, "/activities/"+strings.TrimPrefix(createTicketActivityAPID, cfg.BaseURL+"/activities/"))
	require.Equal(t, "Create", createTicketActivityDoc["type"])
	requireInboxItem(t, db, project.ID, "Create", createdTicket.APID)
	requireOutboxItem(t, db, owner.ID, "Create", createdTicket.APID)
	requireDeliveryForObject(t, db, "Create", createdTicket.APID, remoteInbox)

	c2sTicketAPID := requireC2SCreateTicket(t, db, cfg, ticketService, commentService, owner.ID, owner.Username, owner.APID, project.APID, remoteInbox)
	requireProjectTicketsCollectionContains(t, db, cfg, project.ID, project.APID, c2sTicketAPID)

	status := "done"
	assigneeID := member.ID
	assigneePatch := &assigneeID
	require.NoError(t, ticketService.UpdateTicket(ctx, ticket.UpdateTicketRequest{
		Status:     &status,
		AssigneeID: &assigneePatch,
	}, createdTicket.ID, member.ID))
	requireActivityForObject(t, db, "Update", createdTicket.APID)
	requireActivityForObjectAndActor(t, db, "Update", createdTicket.APID, member.ID)
	requireActivityForObjectAndTarget(t, db, "Add", member.APID, createdTicket.APID)
	requireActivityForObjectAndActor(t, db, "Add", member.APID, member.ID)
	requireOutboxItem(t, db, member.ID, "Update", createdTicket.APID)
	requireDeliveryForObject(t, db, "Update", createdTicket.APID, remoteInbox)
	requireTicketResolved(t, db, createdTicket.ID)
	requireTicketResolvedBy(t, db, createdTicket.ID, member.ID)

	createdComment, err := commentService.CreateComment(ctx, createdTicket.ID, member.ID, "This is ready.")
	require.NoError(t, err)
	requireObjectType(t, db, createdComment.APID, "Note")
	requireActivityForObject(t, db, "Create", createdComment.APID)
	requireOutboxItem(t, db, member.ID, "Create", createdComment.APID)
	requireDeliveryForObject(t, db, "Create", createdComment.APID, remoteInbox)
	requireProjectActivityPubCollections(t, db, cfg, project.ID, project.APID, createdTicket.APID, remoteFollower.APID)

	ownerDeliveries, err := deliveryService.ListProjectDeliveries(ctx, project.ID, owner.ID)
	require.NoError(t, err)
	requireProjectDelivery(t, ownerDeliveries, "Create", createdTicket.APID, remoteInbox)
	requireProjectDelivery(t, ownerDeliveries, "Update", createdTicket.APID, remoteInbox)
	requireProjectDelivery(t, ownerDeliveries, "Create", createdComment.APID, remoteInbox)

	memberDeliveries, err := deliveryService.ListProjectDeliveries(ctx, project.ID, member.ID)
	require.NoError(t, err)
	requireProjectDelivery(t, memberDeliveries, "Create", createdTicket.APID, remoteInbox)

	require.NoError(t, commentService.DeleteComment(ctx, createdComment.ID, owner.ID))
	requireObjectDeleted(t, db, createdComment.APID)
	requireObjectTombstone(t, db, createdComment.APID, "Note")
	requireActivityForObject(t, db, "Delete", createdComment.APID)
	requireOutboxItem(t, db, owner.ID, "Delete", createdComment.APID)
	requireDeliveryForObject(t, db, "Delete", createdComment.APID, remoteInbox)

	comments, err := commentService.ListComments(ctx, createdTicket.ID, owner.ID)
	require.NoError(t, err)
	require.Empty(t, comments)

	c2sCommentAPID := requireC2SCreateNote(t, db, cfg, ticketService, commentService, owner.ID, owner.Username, owner.APID, createdTicket.APID, project.ID, remoteInbox)
	requireC2SOutboxRejectsActorMismatch(t, db, cfg, ticketService, commentService, owner.ID, owner.Username, member.APID, createdTicket.APID)

	ticketComment, err := commentService.CreateComment(ctx, createdTicket.ID, owner.ID, "Deleting this with the ticket.")
	require.NoError(t, err)

	require.NoError(t, ticketService.DeleteTicket(ctx, createdTicket.ID, owner.ID))
	requireObjectDeleted(t, db, createdTicket.APID)
	requireObjectTombstone(t, db, createdTicket.APID, "forge:Ticket")
	requireObjectDeleted(t, db, ticketComment.APID)
	requireObjectTombstone(t, db, ticketComment.APID, "Note")
	requireObjectDeleted(t, db, c2sCommentAPID)
	requireObjectTombstone(t, db, c2sCommentAPID, "Note")
	requireActivityForObject(t, db, "Delete", createdTicket.APID)
	requireOutboxItem(t, db, owner.ID, "Delete", createdTicket.APID)
	requireDeliveryForObject(t, db, "Delete", createdTicket.APID, remoteInbox)
	requireNoTicketByAPID(t, db, createdTicket.APID)
	requireProjectTicketsCollectionNotContains(t, db, cfg, project.ID, project.APID, createdTicket.APID)

	outsider, err := userService.RegisterUser(ctx, "outsider", "outsider@example.test", "password123")
	require.NoError(t, err)
	_, err = ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
		Title:       "Invalid assignee",
		Description: "Should not assign a non-participant",
		Priority:    "medium",
		Type:        "task",
		AssigneeID:  &outsider.ID,
	}, project.ID, owner.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "assignee must be a project participant")

	assignmentTarget, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
		Title:       "Assignment guard target",
		Description: "Used for update permission validation",
		Priority:    "medium",
		Type:        "task",
	}, project.ID, owner.ID)
	require.NoError(t, err)
	outsiderPatch := &outsider.ID
	err = ticketService.UpdateTicket(ctx, ticket.UpdateTicketRequest{AssigneeID: &outsiderPatch}, assignmentTarget.ID, owner.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "assignee must be a project participant")

	viewer, err := userService.RegisterUser(ctx, "viewer", "viewer@example.test", "password123")
	require.NoError(t, err)
	viewerInvite, err := projectService.AddMemberToProject(ctx, project.ID, owner.ID, viewer.ID, "viewer")
	require.NoError(t, err)
	require.NoError(t, projectService.AcceptInvite(ctx, viewerInvite.ID, viewer.ID))
	requireProjectRole(t, db, viewer.ID, project.ID, "viewer")

	deniedActivityCount := activityCount(t, db)
	_, err = ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
		Title:       "Viewer ticket",
		Description: "Should not be written",
		Priority:    "medium",
		Type:        "task",
	}, project.ID, viewer.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "viewers cannot create tickets")

	memberPatch := &member.ID
	err = ticketService.UpdateTicket(ctx, ticket.UpdateTicketRequest{AssigneeID: &memberPatch}, assignmentTarget.ID, viewer.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "viewers cannot update tickets")

	err = ticketService.DeleteTicket(ctx, assignmentTarget.ID, viewer.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "only owners or managers can delete tickets")

	_, err = commentService.CreateComment(ctx, assignmentTarget.ID, viewer.ID, "Viewer comment should not publish.")
	require.Error(t, err)
	require.Contains(t, err.Error(), "viewers cannot comment")

	newProjectName := "Viewer renamed board"
	err = projectService.UpdateProject(ctx, project.ID, viewer.ID, projectUpdateNameRequest(&newProjectName))
	require.Error(t, err)
	require.Contains(t, err.Error(), "only owners or managers can update projects")

	err = projectService.DeleteProject(ctx, project.ID, viewer.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "only owners can delete projects")
	requireActivityCount(t, db, deniedActivityCount)

	manager, err := userService.RegisterUser(ctx, "manager", "manager@example.test", "password123")
	require.NoError(t, err)
	managerInvite, err := projectService.AddMemberToProject(ctx, project.ID, owner.ID, manager.ID, "manager")
	require.NoError(t, err)
	require.NoError(t, projectService.AcceptInvite(ctx, managerInvite.ID, manager.ID))
	requireProjectRole(t, db, manager.ID, project.ID, "manager")
	requireDeliveryForObject(t, db, "Accept", managerInvite.APID, remoteInbox)

	updatedProjectName := "Federated Board Renamed"
	updatedProjectDescription := "A manager-published ActivityPub board update"
	updateRequest := projectUpdateNameRequest(&updatedProjectName)
	updateRequest.Description = &updatedProjectDescription
	require.NoError(t, projectService.UpdateProject(ctx, project.ID, manager.ID, updateRequest))
	requireProjectObjectFields(t, db, project.APID, updatedProjectName, updatedProjectDescription)
	requireActivityForObjectAndActor(t, db, "Update", project.APID, manager.ID)
	requireActivityForObjectAndTarget(t, db, "Update", project.APID, project.APID)
	requireOutboxItem(t, db, manager.ID, "Update", project.APID)
	requireInboxItem(t, db, project.ID, "Update", project.APID)
	requireDeliveryForObject(t, db, "Update", project.APID, remoteInbox)
	managerDeliveries, err := deliveryService.ListProjectDeliveries(ctx, project.ID, manager.ID)
	require.NoError(t, err)
	requireProjectDelivery(t, managerDeliveries, "Update", project.APID, remoteInbox)
	project.Name = updatedProjectName
	project.Description = updatedProjectDescription

	removableDev, err := userService.RegisterUser(ctx, "removable-dev", "removable-dev@example.test", "password123")
	require.NoError(t, err)
	removableInvite, err := projectService.AddMemberToProject(ctx, project.ID, owner.ID, removableDev.ID, "developer")
	require.NoError(t, err)
	require.NoError(t, projectService.AcceptInvite(ctx, removableInvite.ID, removableDev.ID))
	requireProjectRole(t, db, removableDev.ID, project.ID, "developer")

	memberRemovalTicket, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
		Title:       "Remove assignee with member",
		Description: "Membership removal should clear assignment.",
		Priority:    "medium",
		Type:        "task",
		AssigneeID:  &removableDev.ID,
	}, project.ID, owner.ID)
	require.NoError(t, err)
	requireTicketAssignee(t, db, memberRemovalTicket.APID, removableDev.ID)
	requireTicketObjectAssignedTo(t, db, memberRemovalTicket.APID, removableDev.APID)

	managerDeniedRemovalCount := activityCount(t, db)
	err = projectService.RemoveMemberFromProject(ctx, project.ID, manager.ID, owner.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "managers can only remove")
	requireActivityCount(t, db, managerDeniedRemovalCount)

	require.NoError(t, projectService.RemoveMemberFromProject(ctx, project.ID, manager.ID, removableDev.ID))
	requireNoProjectMember(t, db, removableDev.ID, project.ID)
	requireNoFollow(t, db, removableDev.ID, project.ID)
	requireNoTicketAssignee(t, db, memberRemovalTicket.APID, removableDev.ID)
	requireTicketObjectNotAssignedTo(t, db, memberRemovalTicket.APID, removableDev.APID)
	requireActivityForObjectAndTarget(t, db, "Remove", removableDev.APID, project.APID)
	requireActivityForObjectAndActor(t, db, "Remove", removableDev.APID, manager.ID)
	requireInboxItem(t, db, project.ID, "Remove", removableDev.APID)
	requireInboxItem(t, db, removableDev.ID, "Remove", removableDev.APID)
	requireDeliveryForObject(t, db, "Remove", removableDev.APID, remoteInbox)
	_, err = projectService.GetProjectByID(ctx, project.ID, removableDev.ID)
	require.Error(t, err)

	lastOwnerRemovalCount := activityCount(t, db)
	err = projectService.RemoveMemberFromProject(ctx, project.ID, owner.ID, owner.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "last project owner")
	requireActivityCount(t, db, lastOwnerRemovalCount)

	require.NoError(t, projectService.RemoveMemberFromProject(ctx, project.ID, viewer.ID, viewer.ID))
	requireNoProjectMember(t, db, viewer.ID, project.ID)
	requireNoFollow(t, db, viewer.ID, project.ID)
	requireActivityForObjectAndTarget(t, db, "Undo", project.APID, project.APID)
	requireActivityForObjectAndActor(t, db, "Undo", project.APID, viewer.ID)
	requireOutboxItem(t, db, viewer.ID, "Undo", project.APID)
	requireDeliveryForObject(t, db, "Undo", project.APID, remoteInbox)
	_, err = projectService.GetProjectByID(ctx, project.ID, viewer.ID)
	require.Error(t, err)

	_, err = deliveryService.ListProjectDeliveries(ctx, project.ID, outsider.ID)
	require.ErrorIs(t, err, delivery.ErrProjectAccessDenied)

	projectDeleteComment, err := commentService.CreateComment(ctx, assignmentTarget.ID, owner.ID, "Project delete should tombstone this note.")
	require.NoError(t, err)

	forbiddenDeleteCount := activityCount(t, db)
	err = projectService.DeleteProject(ctx, project.ID, manager.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "only owners can delete projects")
	requireActivityCount(t, db, forbiddenDeleteCount)

	require.NoError(t, projectService.DeleteProject(ctx, project.ID, owner.ID))
	requireObjectDeleted(t, db, project.APID)
	requireObjectTombstone(t, db, project.APID, "Group")
	requireObjectDeleted(t, db, assignmentTarget.APID)
	requireObjectTombstone(t, db, assignmentTarget.APID, "forge:Ticket")
	requireObjectDeleted(t, db, memberRemovalTicket.APID)
	requireObjectTombstone(t, db, memberRemovalTicket.APID, "forge:Ticket")
	requireObjectDeleted(t, db, projectDeleteComment.APID)
	requireObjectTombstone(t, db, projectDeleteComment.APID, "Note")
	requireActivityForObjectAndActor(t, db, "Delete", project.APID, owner.ID)
	requireActivityForObjectAndTarget(t, db, "Delete", project.APID, project.APID)
	requireOutboxItem(t, db, owner.ID, "Delete", project.APID)
	requireInboxItem(t, db, project.ID, "Delete", project.APID)
	requireDeliveryForObject(t, db, "Delete", project.APID, remoteInbox)
	postDeleteDeliveries, err := deliveryService.ListProjectDeliveries(ctx, project.ID, owner.ID)
	require.NoError(t, err)
	projectDeleteDelivery := findProjectDelivery(postDeleteDeliveries, "Delete", project.APID, remoteInbox)
	require.NotNil(t, projectDeleteDelivery)
	require.NoError(t, delivery.NewRepository(db).MarkFailed(ctx, projectDeleteDelivery.ID, "remote server unavailable", delivery.FailureDetails{Kind: delivery.FailureKindNetwork}, nil))
	deadProjectDeliveries, err := deliveryService.ListProjectDeliveriesWithOptions(ctx, project.ID, owner.ID, delivery.ProjectDeliveryListOptions{
		State: delivery.StateDead,
	})
	require.NoError(t, err)
	requireProjectDelivery(t, deadProjectDeliveries, "Delete", project.APID, remoteInbox)
	projectDeliverySummary, err := deliveryService.GetProjectDeliverySummary(ctx, project.ID, owner.ID)
	require.NoError(t, err)
	require.True(t, projectDeliverySummary.CanRetry)
	require.GreaterOrEqual(t, projectDeliverySummary.Dead, 1)
	retriedProjectDelivery, err := deliveryService.RetryProjectDelivery(ctx, project.ID, owner.ID, projectDeleteDelivery.ID)
	require.NoError(t, err)
	assert.Equal(t, delivery.StatePending, retriedProjectDelivery.State)
	_, err = deliveryService.ListProjectDeliveries(ctx, project.ID, manager.ID)
	require.ErrorIs(t, err, delivery.ErrProjectAccessDenied)
	requireNoProjectByID(t, db, project.ID)
	requireNoTicketByAPID(t, db, assignmentTarget.APID)
	requireNoTicketByAPID(t, db, memberRemovalTicket.APID)
	requireNoFollow(t, db, owner.ID, project.ID)
	requireNoFollow(t, db, member.ID, project.ID)
	requireNoFollow(t, db, manager.ID, project.ID)
	requireNoFollow(t, db, remoteFollower.ID, project.ID)
	_, err = projectService.GetProjectByID(ctx, project.ID, owner.ID)
	require.Error(t, err)
	requireProjectActorEndpointTombstone(t, db, cfg, project.ID, "Group")
}

func TestActivityPubReadAuthorization(t *testing.T) {
	db := openIntegrationDB(t)
	resetIntegrationDB(t, db)

	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	jwtSecret := []byte("integration-secret")

	userService := user.NewService(user.NewRepository(db, cfg), jwtSecret, cfg)
	projectService := project.NewService(project.NewRepository(db, cfg), cfg)
	ticketService := ticket.NewService(ticket.NewRepository(db, cfg), projectService, cfg)
	commentService := comment.NewService(comment.NewRepository(db, cfg), ticketService, cfg)
	deliveryService := delivery.NewService(delivery.NewRecipientRepository(db), delivery.NoopQueue{})
	projectService.SetDelivery(deliveryService)
	ticketService.SetDelivery(deliveryService)
	commentService.SetDelivery(deliveryService)

	owner, err := userService.RegisterUser(ctx, "auth-owner", "auth-owner@example.test", "password123")
	require.NoError(t, err)
	outsider, err := userService.RegisterUser(ctx, "auth-outsider", "auth-outsider@example.test", "password123")
	require.NoError(t, err)
	createdProject, err := projectService.CreateProject(ctx, "Protected AP Board", "", owner.ID)
	require.NoError(t, err)
	createdTicket, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
		Title:       "Protect ActivityPub reads",
		Description: "ActivityPub object reads should respect project access.",
		Priority:    "medium",
		Type:        "task",
	}, createdProject.ID, owner.ID)
	require.NoError(t, err)
	createdComment, err := commentService.CreateComment(ctx, createdTicket.ID, owner.ID, "Only project participants should read this.")
	require.NoError(t, err)

	ticketCreateActivityAPID := requireActivityAPIDForObject(t, db, "Create", createdTicket.APID)
	ticketCreateActivityID := strings.TrimPrefix(ticketCreateActivityAPID, cfg.BaseURL+"/activities/")
	ownerToken, err := userService.Login(ctx, owner.Email, "password123")
	require.NoError(t, err)
	outsiderToken, err := userService.Login(ctx, outsider.Email, "password123")
	require.NoError(t, err)

	remoteFollower, remoteFollowerPrivateKey := createRemoteActor(t, ctx, db, "auth-follower")
	_, err = db.ExecContext(ctx, `
		INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
		VALUES ($1, $2, 'accepted')
	`, remoteFollower.ID, createdProject.ID)
	require.NoError(t, err)
	remoteStranger, remoteStrangerPrivateKey := createRemoteActor(t, ctx, db, "auth-stranger")

	e := echo.New()
	activitypub.NewHandlerWithAuthorizer(
		db,
		cfg,
		activitypub.NewAccessAuthorizer(
			db,
			jwtSecret,
			integrationSignatureActorVerifier{service: httpsig.NewService(httpsig.NewRepository(db))},
		),
	).RegisterRoutes(e)

	for _, path := range []string{
		"/users/" + owner.Username,
		"/projects/" + createdProject.ID,
	} {
		requireActivityPubReadStatus(t, e, path, "", nil, http.StatusOK)
	}

	for _, path := range []string{
		"/tickets/" + createdTicket.ID,
		"/comments/" + createdComment.ID,
		"/activities/" + ticketCreateActivityID,
		"/projects/" + createdProject.ID + "/outbox?page=true",
		"/projects/" + createdProject.ID + "/tickets?page=true",
	} {
		requireActivityPubReadStatus(t, e, path, "", nil, http.StatusUnauthorized)
		requireActivityPubReadStatus(t, e, path, "Bearer "+ownerToken, nil, http.StatusOK)
		requireActivityPubReadStatus(t, e, path, "Bearer "+outsiderToken, nil, http.StatusForbidden)
	}

	for _, path := range []string{
		"/tickets/" + createdTicket.ID,
		"/projects/" + createdProject.ID + "/tickets?page=true",
	} {
		requireActivityPubReadStatus(t, e, path, "", func(req *http.Request) {
			signRemoteInboxRequest(t, ctx, req, remoteFollower, remoteFollowerPrivateKey, nil)
		}, http.StatusOK)
		requireActivityPubReadStatus(t, e, path, "", func(req *http.Request) {
			signRemoteInboxRequest(t, ctx, req, remoteStranger, remoteStrangerPrivateKey, nil)
		}, http.StatusForbidden)
	}
}

func TestActivityPubFoundationConstraints(t *testing.T) {
	db := openIntegrationDB(t)
	resetIntegrationDB(t, db)

	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")

	userRepo := user.NewRepository(db, cfg)
	userService := user.NewService(userRepo, []byte("integration-secret"), cfg)
	projectRepo := project.NewRepository(db, cfg)
	projectService := project.NewService(projectRepo, cfg)

	owner, err := userService.RegisterUser(ctx, "owner", "owner@example.test", "password123")
	require.NoError(t, err)
	project, err := projectService.CreateProject(ctx, "Constraint Board", "", owner.ID)
	require.NoError(t, err)

	t.Run("core tables exist", func(t *testing.T) {
		for _, tableName := range []string{
			"actors",
			"actor_keys",
			"ap_objects",
			"ap_activities",
			"activity_deliveries",
			"actor_inbox_items",
			"actor_outbox_items",
			"federation_domain_blocks",
			"project_members",
			"actor_follows",
			"project_invites",
			"tickets",
			"ticket_assignees",
			"comments",
		} {
			var exists bool
			err := db.Get(&exists, `SELECT to_regclass($1) IS NOT NULL`, tableName)
			require.NoError(t, err)
			assert.True(t, exists, tableName)
		}
	})

	t.Run("duplicate actor handle fails", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
			INSERT INTO actors (
				ap_id, type, preferred_username, handle, name, inbox_url, outbox_url, followers_url
			)
			VALUES (
				$1, 'Person', 'owner-copy', $2, 'Owner Copy', $3, $4, $5
			)
		`,
			"http://localhost:8080/users/owner-copy",
			owner.Handle,
			"http://localhost:8080/users/owner-copy/inbox",
			"http://localhost:8080/users/owner-copy/outbox",
			"http://localhost:8080/users/owner-copy/followers",
		)
		require.Error(t, err)
	})

	t.Run("invalid ticket status fails", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
			INSERT INTO tickets (
				ap_id, project_id, reporter_id, title, status, priority, type
			)
			VALUES ($1, $2, $3, 'Invalid status', 'blocked', 'medium', 'task')
		`, "http://localhost:8080/tickets/invalid-status", project.ID, owner.ID)
		require.Error(t, err)
	})

	t.Run("remote service actor allows public only key", func(t *testing.T) {
		repo := remoteactor.NewRepository(db)
		followersURL := "https://remote.example/users/bot/followers"
		actor := &remoteactor.Actor{
			APID:              "https://remote.example/users/bot",
			Type:              "Service",
			PreferredUsername: "bot",
			Handle:            "bot@remote.example",
			Name:              "Remote Bot",
			Summary:           "",
			InboxURL:          "https://remote.example/users/bot/inbox",
			OutboxURL:         "https://remote.example/users/bot/outbox",
			FollowersURL:      &followersURL,
			PublicKeyID:       "https://remote.example/users/bot#main-key",
			PublicKeyPEM:      "public key",
			Document:          []byte(`{"id":"https://remote.example/users/bot","type":"Service"}`),
		}

		require.NoError(t, repo.UpsertRemoteActor(ctx, actor))

		var isLocal bool
		require.NoError(t, db.GetContext(ctx, &isLocal, `
			SELECT is_local FROM actors WHERE ap_id = $1
		`, actor.APID))
		assert.False(t, isLocal)

		var privateKey *string
		require.NoError(t, db.GetContext(ctx, &privateKey, `
			SELECT private_key_pem FROM actor_keys WHERE key_id = $1
		`, actor.PublicKeyID))
		assert.Nil(t, privateKey)

		var fetched struct {
			LastFetchedAt sql.NullTime   `db:"last_fetched_at"`
			FetchError    sql.NullString `db:"fetch_error"`
			FetchErrorAt  sql.NullTime   `db:"fetch_error_at"`
		}
		require.NoError(t, db.GetContext(ctx, &fetched, `
			SELECT last_fetched_at, fetch_error, fetch_error_at
			FROM actors
			WHERE ap_id = $1
		`, actor.APID))
		require.True(t, fetched.LastFetchedAt.Valid)
		require.False(t, fetched.FetchError.Valid)
		require.False(t, fetched.FetchErrorAt.Valid)

		require.NoError(t, repo.RecordRemoteActorFetchFailure(ctx, actor.APID, "remote actor fetch failed"))
		require.NoError(t, db.GetContext(ctx, &fetched, `
			SELECT last_fetched_at, fetch_error, fetch_error_at
			FROM actors
			WHERE ap_id = $1
		`, actor.APID))
		require.True(t, fetched.LastFetchedAt.Valid)
		require.True(t, fetched.FetchError.Valid)
		require.True(t, fetched.FetchErrorAt.Valid)
		assert.Equal(t, "remote actor fetch failed", fetched.FetchError.String)

		actor.Name = "Remote Bot Refreshed"
		require.NoError(t, repo.UpsertRemoteActor(ctx, actor))
		require.NoError(t, db.GetContext(ctx, &fetched, `
			SELECT last_fetched_at, fetch_error, fetch_error_at
			FROM actors
			WHERE ap_id = $1
		`, actor.APID))
		require.True(t, fetched.LastFetchedAt.Valid)
		require.False(t, fetched.FetchError.Valid)
		require.False(t, fetched.FetchErrorAt.Valid)
	})

	t.Run("remote actor key rotation deactivates old key", func(t *testing.T) {
		repo := remoteactor.NewRepository(db)
		oldPublicKey, _, err := activitypub.GenerateRSAKeyPair()
		require.NoError(t, err)
		newPublicKey, _, err := activitypub.GenerateRSAKeyPair()
		require.NoError(t, err)

		actorAPID := "https://remote.example/users/rotating-key"
		actor := &remoteactor.Actor{
			APID:              actorAPID,
			Type:              "Person",
			PreferredUsername: "rotating-key",
			Handle:            "rotating-key@remote.example",
			Name:              "Rotating Key",
			InboxURL:          activitypub.Inbox(actorAPID),
			OutboxURL:         activitypub.Outbox(actorAPID),
			PublicKeyID:       actorAPID + "#old-key",
			PublicKeyPEM:      oldPublicKey,
			Document:          []byte(`{"id":"https://remote.example/users/rotating-key","type":"Person"}`),
		}
		require.NoError(t, repo.UpsertRemoteActor(ctx, actor))

		actor.PublicKeyID = actorAPID + "#new-key"
		actor.PublicKeyPEM = newPublicKey
		require.NoError(t, repo.UpsertRemoteActor(ctx, actor))

		var activeOld bool
		require.NoError(t, db.GetContext(ctx, &activeOld, `SELECT active FROM actor_keys WHERE key_id = $1`, actorAPID+"#old-key"))
		assert.False(t, activeOld)
		requireActorPublicKey(t, db, actorAPID+"#new-key", newPublicKey)
	})

	t.Run("remote actors can author tickets and comments", func(t *testing.T) {
		remoteActor := &remoteactor.Actor{
			APID:              "https://remote.example/users/author",
			Type:              "Person",
			PreferredUsername: "author",
			Handle:            "author@remote.example",
			Name:              "Remote Author",
			Summary:           "",
			InboxURL:          "https://remote.example/users/author/inbox",
			OutboxURL:         "https://remote.example/users/author/outbox",
			PublicKeyID:       "https://remote.example/users/author#main-key",
			PublicKeyPEM:      "public key",
			Document:          []byte(`{"id":"https://remote.example/users/author","type":"Person"}`),
		}
		require.NoError(t, remoteactor.NewRepository(db).UpsertRemoteActor(ctx, remoteActor))

		ticketID, err := activitypub.NewID()
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `
			INSERT INTO tickets (
				id, ap_id, project_id, reporter_id, title, status, priority, type
			)
			VALUES ($1, $2, $3, $4, 'Remote authored ticket', 'open', 'medium', 'task')
		`, ticketID, activitypub.TicketAPID(cfg, ticketID), project.ID, remoteActor.ID)
		require.NoError(t, err)

		commentID, err := activitypub.NewID()
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `
			INSERT INTO comments (id, ap_id, ticket_id, author_id, content)
			VALUES ($1, $2, $3, $4, 'Remote authored comment')
		`, commentID, activitypub.CommentAPID(cfg, commentID), ticketID, remoteActor.ID)
		require.NoError(t, err)
	})

	t.Run("remote inbox stores inbound activity idempotently", func(t *testing.T) {
		publicKey, privateKey, err := activitypub.GenerateRSAKeyPair()
		require.NoError(t, err)

		remoteActorRepo := remoteactor.NewRepository(db)
		remoteActor := &remoteactor.Actor{
			APID:              "https://remote.example/users/inbox-bot",
			Type:              "Service",
			PreferredUsername: "inbox-bot",
			Handle:            "inbox-bot@remote.example",
			Name:              "Inbox Bot",
			Summary:           "",
			InboxURL:          "https://remote.example/users/inbox-bot/inbox",
			OutboxURL:         "https://remote.example/users/inbox-bot/outbox",
			PublicKeyID:       "https://remote.example/users/inbox-bot#main-key",
			PublicKeyPEM:      publicKey,
			Document:          []byte(`{"id":"https://remote.example/users/inbox-bot","type":"Service"}`),
		}
		require.NoError(t, remoteActorRepo.UpsertRemoteActor(ctx, remoteActor))

		inboxRepo := remoteinbox.NewRepository(db)
		objectAPID := "https://remote.example/notes/1"
		body := []byte(`{"id":"https://remote.example/activities/inbox-create","type":"Create","actor":"https://remote.example/users/inbox-bot","object":"https://remote.example/notes/1"}`)
		req, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(body))
		require.NoError(t, err)

		signer := httpsig.NewService(singleKeyRepo{key: &httpsig.ActorKey{
			ActorID:       remoteActor.ID,
			ActorAPID:     remoteActor.APID,
			KeyID:         remoteActor.PublicKeyID,
			Algorithm:     httpsig.AlgorithmRSAV15SHA256,
			PublicKeyPEM:  publicKey,
			PrivateKeyPEM: privateKey,
		}})
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, req, body))

		receiver := remoteinbox.NewService(inboxRepo, httpsig.NewService(httpsig.NewRepository(db)))
		first, err := receiver.Receive(ctx, req, project.APID, body)
		require.NoError(t, err)
		assert.False(t, first.Duplicate)

		second, err := receiver.Receive(ctx, req, project.APID, body)
		require.NoError(t, err)
		assert.True(t, second.Duplicate)

		requireInboxItem(t, db, project.ID, "Create", objectAPID)
	})

	t.Run("remote inbox resolves unknown actor key", func(t *testing.T) {
		publicKey, privateKey, err := activitypub.GenerateRSAKeyPair()
		require.NoError(t, err)

		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/users/unknown-key" {
				http.NotFound(w, r)
				return
			}

			doc := activitypub.ActorDocument(
				"Person",
				server.URL+"/users/unknown-key",
				"unknown-key",
				"Unknown Key",
				"Resolved during signature verification.",
				publicKey,
			)
			rawDoc, err := activitypub.MarshalDocument(doc)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/activity+json")
			_, _ = w.Write(rawDoc)
		}))
		defer server.Close()

		actorAPID := server.URL + "/users/unknown-key"
		followAPID := server.URL + "/activities/follow-unknown-key-project"
		body := []byte(`{"id":"` + followAPID + `","type":"Follow","actor":"` + actorAPID + `","object":"` + project.APID + `"}`)
		req, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(body))
		require.NoError(t, err)

		keyID := activitypub.KeyID(actorAPID)
		signer := httpsig.NewService(singleKeyRepo{key: &httpsig.ActorKey{
			ActorID:       "remote-unknown-key-before-cache",
			ActorAPID:     actorAPID,
			KeyID:         keyID,
			Algorithm:     httpsig.AlgorithmRSAV15SHA256,
			PublicKeyPEM:  publicKey,
			PrivateKeyPEM: privateKey,
		}})
		require.NoError(t, signer.SignRequest(ctx, "remote-unknown-key-before-cache", req, body))

		remoteActorService := remoteactor.NewService(
			remoteactor.NewRepository(db),
			remoteactor.WithHTTPClient(server.Client()),
		)
		receiver := remoteinbox.NewService(
			remoteinbox.NewRepository(db, cfg),
			httpsig.NewService(
				httpsig.NewRepository(db),
				httpsig.WithMissingKeyResolver(remoteActorService.ResolveKey),
			),
		)

		accepted, err := receiver.Receive(ctx, req, project.APID, body)
		require.NoError(t, err)
		require.Equal(t, followAPID, accepted.ActivityAPID)
		require.Empty(t, accepted.ResponseActivityID)

		var storedActorID string
		require.NoError(t, db.GetContext(ctx, &storedActorID, `
			SELECT id::text
			FROM actors
			WHERE ap_id = $1 AND is_local = false
		`, actorAPID))
		requireNoFollow(t, db, storedActorID, project.ID)
		requireActivityForObject(t, db, "Follow", project.APID)
		requireInboxItem(t, db, project.ID, "Follow", project.APID)
	})

	t.Run("remote inbox accepts invited project actor mutations and fans out", func(t *testing.T) {
		publicKey, privateKey, err := activitypub.GenerateRSAKeyPair()
		require.NoError(t, err)

		remoteActor := &remoteactor.Actor{
			APID:              "https://remote.example/users/follow-bot",
			Type:              "Person",
			PreferredUsername: "follow-bot",
			Handle:            "follow-bot@remote.example",
			Name:              "Follow Bot",
			Summary:           "",
			InboxURL:          "https://remote.example/users/follow-bot/inbox",
			OutboxURL:         "https://remote.example/users/follow-bot/outbox",
			PublicKeyID:       "https://remote.example/users/follow-bot#main-key",
			PublicKeyPEM:      publicKey,
			Document:          []byte(`{"id":"https://remote.example/users/follow-bot","type":"Person"}`),
		}
		require.NoError(t, remoteactor.NewRepository(db).UpsertRemoteActor(ctx, remoteActor))

		signer := httpsig.NewService(singleKeyRepo{key: &httpsig.ActorKey{
			ActorID:       remoteActor.ID,
			ActorAPID:     remoteActor.APID,
			KeyID:         remoteActor.PublicKeyID,
			Algorithm:     httpsig.AlgorithmRSAV15SHA256,
			PublicKeyPEM:  publicKey,
			PrivateKeyPEM: privateKey,
		}})

		deliveryService := delivery.NewService(delivery.NewRecipientRepository(db), delivery.NoopQueue{})
		receiver := remoteinbox.NewService(
			remoteinbox.NewRepository(db, cfg),
			httpsig.NewService(httpsig.NewRepository(db)),
			remoteinbox.WithDelivery(deliveryService),
		)

		invite, err := projectService.AddMemberToProject(ctx, project.ID, owner.ID, remoteActor.ID, "developer")
		require.NoError(t, err)
		acceptInviteAPID := "https://remote.example/activities/accept-follow-bot-invite"
		acceptInviteBody := []byte(`{"id":"` + acceptInviteAPID + `","type":"Accept","actor":"` + remoteActor.APID + `","object":"` + invite.APID + `","target":"` + project.APID + `"}`)
		acceptInviteReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(acceptInviteBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, acceptInviteReq, acceptInviteBody))
		acceptedInvite, err := receiver.Receive(ctx, acceptInviteReq, project.APID, acceptInviteBody)
		require.NoError(t, err)
		require.Equal(t, acceptInviteAPID, acceptedInvite.ActivityAPID)
		requireInviteStatus(t, db, invite.ID, "accepted")
		requireFollow(t, db, remoteActor.ID, project.ID, "accepted")
		requireActivityForObjectAndActor(t, db, "Accept", invite.APID, remoteActor.ID)
		requireInboxItem(t, db, project.ID, "Accept", invite.APID)

		followAPID := "https://remote.example/activities/follow-project"
		body := []byte(`{"id":"` + followAPID + `","type":"Follow","actor":"` + remoteActor.APID + `","object":"` + project.APID + `"}`)
		req, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(body))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, req, body))
		accepted, err := receiver.Receive(ctx, req, project.APID, body)
		require.NoError(t, err)
		require.Empty(t, accepted.ResponseActivityID)
		requireActivityForObject(t, db, "Follow", project.APID)
		requireFollow(t, db, remoteActor.ID, project.ID, "accepted")

		fanoutPeer, _ := createRemoteActor(t, ctx, db, "fanout-peer")
		_, err = db.ExecContext(ctx, `
			INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
			VALUES ($1, $2, 'accepted')
		`, fanoutPeer.ID, project.ID)
		require.NoError(t, err)

		ticketService := ticket.NewService(ticket.NewRepository(db, cfg), projectService, cfg)
		ticketService.SetDelivery(deliveryService)
		localTicket, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
			Title:       "Remote comment target",
			Description: "A local ticket for remote discussion",
			Priority:    "medium",
			Type:        "task",
		}, project.ID, owner.ID)
		require.NoError(t, err)

		noteAPID := "https://remote.example/notes/project-comment"
		createNoteAPID := "https://remote.example/activities/create-project-comment"
		noteBody := []byte(`{"id":"` + createNoteAPID + `","type":"Create","actor":"` + remoteActor.APID + `","object":{"id":"` + noteAPID + `","type":"Note","attributedTo":"` + remoteActor.APID + `","inReplyTo":"` + localTicket.APID + `","content":"Remote review looks good."}}`)
		noteReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(noteBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, noteReq, noteBody))

		remoteComment, err := receiver.Receive(ctx, noteReq, project.APID, noteBody)
		require.NoError(t, err)
		require.Equal(t, createNoteAPID, remoteComment.ActivityAPID)
		requireObjectType(t, db, noteAPID, "Note")
		requireActivityForObject(t, db, "Create", noteAPID)
		requireInboxItem(t, db, project.ID, "Create", noteAPID)
		requireDeliveryForObject(t, db, "Create", noteAPID, fanoutPeer.InboxURL)
		requireNoDeliveryForObject(t, db, "Create", noteAPID, remoteActor.InboxURL)

		commentService := comment.NewService(comment.NewRepository(db, cfg), ticketService, cfg)
		commentService.SetDelivery(deliveryService)
		comments, err := commentService.ListComments(ctx, localTicket.ID, owner.ID)
		require.NoError(t, err)
		require.Len(t, comments, 1)
		assert.Equal(t, noteAPID, comments[0].APID)
		assert.Equal(t, remoteActor.ID, comments[0].AuthorID)
		assert.Equal(t, "Remote review looks good.", comments[0].Content)

		remoteTicketAPID := "https://remote.example/tickets/project-ticket"
		createTicketAPID := "https://remote.example/activities/create-project-ticket"
		ticketBody := []byte(`{"id":"` + createTicketAPID + `","type":"Create","actor":"` + remoteActor.APID + `","object":{"id":"` + remoteTicketAPID + `","type":["forge:Ticket"],"attributedTo":"` + remoteActor.APID + `","context":"` + project.APID + `","name":"Remote ticket","content":"Created from another server.","forge:priority":"urgent","forge:ticketType":"task","forge:isResolved":false}}`)
		ticketReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(ticketBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, ticketReq, ticketBody))

		remoteTicket, err := receiver.Receive(ctx, ticketReq, project.APID, ticketBody)
		require.NoError(t, err)
		require.Equal(t, createTicketAPID, remoteTicket.ActivityAPID)
		requireObjectType(t, db, remoteTicketAPID, "Ticket")
		requireActivityForObject(t, db, "Create", remoteTicketAPID)
		requireInboxItem(t, db, project.ID, "Create", remoteTicketAPID)
		requireDeliveryForObjectFromActor(t, db, "Create", remoteTicketAPID, fanoutPeer.InboxURL, project.ID)
		requireNoDeliveryForObject(t, db, "Create", remoteTicketAPID, remoteActor.InboxURL)

		duplicateRemoteTicket, err := receiver.Receive(ctx, ticketReq, project.APID, ticketBody)
		require.NoError(t, err)
		require.True(t, duplicateRemoteTicket.Duplicate)
		requireDeliveryCountForObject(t, db, "Create", remoteTicketAPID, fanoutPeer.InboxURL, 1)

		tickets, err := ticketService.ListTicketsInProject(ctx, project.ID, owner.ID)
		require.NoError(t, err)
		createdRemoteTicket := findTicketByAPID(tickets, remoteTicketAPID)
		require.NotNil(t, createdRemoteTicket)
		assert.Equal(t, remoteActor.ID, createdRemoteTicket.ReporterID)
		assert.Equal(t, "Remote ticket", createdRemoteTicket.Title)
		assert.Equal(t, "Created from another server.", createdRemoteTicket.Description)
		assert.Equal(t, "urgent", createdRemoteTicket.Priority)
		assert.Equal(t, "task", createdRemoteTicket.Type)

		updateTicketAPID := "https://remote.example/activities/update-project-ticket"
		updateTicketBody := []byte(`{"id":"` + updateTicketAPID + `","type":"Update","actor":"` + remoteActor.APID + `","target":"` + project.APID + `","object":{"id":"` + remoteTicketAPID + `","type":"forge:Ticket","attributedTo":"` + remoteActor.APID + `","context":"` + project.APID + `","name":"Updated remote ticket","content":"Resolved from another server.","forge:status":"done","forge:priority":"high","forge:ticketType":"task","forge:isResolved":true}}`)
		updateTicketReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(updateTicketBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, updateTicketReq, updateTicketBody))

		updatedRemoteTicketActivity, err := receiver.Receive(ctx, updateTicketReq, project.APID, updateTicketBody)
		require.NoError(t, err)
		require.Equal(t, updateTicketAPID, updatedRemoteTicketActivity.ActivityAPID)
		requireObjectType(t, db, remoteTicketAPID, "Ticket")
		requireActivityForObject(t, db, "Update", remoteTicketAPID)
		requireInboxItem(t, db, project.ID, "Update", remoteTicketAPID)
		requireDeliveryForObject(t, db, "Update", remoteTicketAPID, fanoutPeer.InboxURL)
		requireNoDeliveryForObject(t, db, "Update", remoteTicketAPID, remoteActor.InboxURL)

		tickets, err = ticketService.ListTicketsInProject(ctx, project.ID, owner.ID)
		require.NoError(t, err)
		updatedRemoteTicket := findTicketByAPID(tickets, remoteTicketAPID)
		require.NotNil(t, updatedRemoteTicket)
		assert.Equal(t, remoteActor.ID, updatedRemoteTicket.ReporterID)
		assert.Equal(t, "Updated remote ticket", updatedRemoteTicket.Title)
		assert.Equal(t, "Resolved from another server.", updatedRemoteTicket.Description)
		assert.Equal(t, "done", updatedRemoteTicket.Status)
		assert.Equal(t, "high", updatedRemoteTicket.Priority)
		assert.True(t, updatedRemoteTicket.IsResolved)
		requireTicketResolved(t, db, updatedRemoteTicket.ID)

		assignee, err := userService.RegisterUser(ctx, "assignee", "assignee@example.test", "password123")
		require.NoError(t, err)
		assigneeInvite, err := projectService.AddMemberToProject(ctx, project.ID, owner.ID, assignee.ID, "developer")
		require.NoError(t, err)
		require.NoError(t, projectService.AcceptInvite(ctx, assigneeInvite.ID, assignee.ID))

		addAssigneeAPID := "https://remote.example/activities/add-project-ticket-assignee"
		addAssigneeBody := []byte(`{"id":"` + addAssigneeAPID + `","type":"Add","actor":"` + remoteActor.APID + `","object":"` + assignee.APID + `","target":"` + remoteTicketAPID + `"}`)
		addAssigneeReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(addAssigneeBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, addAssigneeReq, addAssigneeBody))

		addedAssignee, err := receiver.Receive(ctx, addAssigneeReq, project.APID, addAssigneeBody)
		require.NoError(t, err)
		require.Equal(t, addAssigneeAPID, addedAssignee.ActivityAPID)
		requireActivityForObjectAndTarget(t, db, "Add", assignee.APID, remoteTicketAPID)
		requireInboxItemForTarget(t, db, project.ID, "Add", remoteTicketAPID)
		requireDeliveryForObject(t, db, "Add", assignee.APID, fanoutPeer.InboxURL)
		requireNoDeliveryForObject(t, db, "Add", assignee.APID, remoteActor.InboxURL)
		requireTicketAssignee(t, db, remoteTicketAPID, assignee.ID)
		requireTicketObjectAssignedTo(t, db, remoteTicketAPID, assignee.APID)

		tickets, err = ticketService.ListTicketsInProject(ctx, project.ID, owner.ID)
		require.NoError(t, err)
		assignedRemoteTicket := findTicketByAPID(tickets, remoteTicketAPID)
		require.NotNil(t, assignedRemoteTicket)
		require.NotNil(t, assignedRemoteTicket.AssigneeID)
		assert.Equal(t, assignee.ID, *assignedRemoteTicket.AssigneeID)

		removeAssigneeAPID := "https://remote.example/activities/remove-project-ticket-assignee"
		removeAssigneeBody := []byte(`{"id":"` + removeAssigneeAPID + `","type":"Remove","actor":"` + remoteActor.APID + `","object":"` + assignee.APID + `","target":"` + remoteTicketAPID + `"}`)
		removeAssigneeReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(removeAssigneeBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, removeAssigneeReq, removeAssigneeBody))

		removedAssignee, err := receiver.Receive(ctx, removeAssigneeReq, project.APID, removeAssigneeBody)
		require.NoError(t, err)
		require.Equal(t, removeAssigneeAPID, removedAssignee.ActivityAPID)
		requireActivityForObjectAndTarget(t, db, "Remove", assignee.APID, remoteTicketAPID)
		requireInboxItemForTarget(t, db, project.ID, "Remove", remoteTicketAPID)
		requireDeliveryForObject(t, db, "Remove", assignee.APID, fanoutPeer.InboxURL)
		requireNoDeliveryForObject(t, db, "Remove", assignee.APID, remoteActor.InboxURL)
		requireNoTicketAssignee(t, db, remoteTicketAPID, assignee.ID)
		requireTicketObjectNotAssignedTo(t, db, remoteTicketAPID, assignee.APID)

		tickets, err = ticketService.ListTicketsInProject(ctx, project.ID, owner.ID)
		require.NoError(t, err)
		unassignedRemoteTicket := findTicketByAPID(tickets, remoteTicketAPID)
		require.NotNil(t, unassignedRemoteTicket)
		assert.Nil(t, unassignedRemoteTicket.AssigneeID)

		remoteTicketLocalComment, err := commentService.CreateComment(ctx, unassignedRemoteTicket.ID, owner.ID, "Local context on remote ticket.")
		require.NoError(t, err)
		requireObjectType(t, db, remoteTicketLocalComment.APID, "Note")

		deleteTicketAPID := "https://remote.example/activities/delete-project-ticket"
		deleteTicketBody := []byte(`{"id":"` + deleteTicketAPID + `","type":"Delete","actor":"` + remoteActor.APID + `","object":"` + remoteTicketAPID + `","target":"` + project.APID + `"}`)
		deleteTicketReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(deleteTicketBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, deleteTicketReq, deleteTicketBody))

		deletedTicket, err := receiver.Receive(ctx, deleteTicketReq, project.APID, deleteTicketBody)
		require.NoError(t, err)
		require.Equal(t, deleteTicketAPID, deletedTicket.ActivityAPID)
		requireActivityForObject(t, db, "Delete", remoteTicketAPID)
		requireInboxItem(t, db, project.ID, "Delete", remoteTicketAPID)
		requireDeliveryForObject(t, db, "Delete", remoteTicketAPID, fanoutPeer.InboxURL)
		requireNoDeliveryForObject(t, db, "Delete", remoteTicketAPID, remoteActor.InboxURL)
		requireObjectDeleted(t, db, remoteTicketAPID)
		requireObjectTombstone(t, db, remoteTicketAPID, "forge:Ticket")
		requireObjectDeleted(t, db, remoteTicketLocalComment.APID)
		requireObjectTombstone(t, db, remoteTicketLocalComment.APID, "Note")
		requireNoTicketByAPID(t, db, remoteTicketAPID)

		tickets, err = ticketService.ListTicketsInProject(ctx, project.ID, owner.ID)
		require.NoError(t, err)
		assert.Nil(t, findTicketByAPID(tickets, remoteTicketAPID))

		undoAPID := "https://remote.example/activities/undo-follow-project"
		undoBody := []byte(`{"id":"` + undoAPID + `","type":"Undo","actor":"` + remoteActor.APID + `","object":{"id":"` + followAPID + `","type":"Follow","actor":"` + remoteActor.APID + `","object":"` + project.APID + `"}}`)
		undoReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(undoBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, undoReq, undoBody))

		undo, err := receiver.Receive(ctx, undoReq, project.APID, undoBody)
		require.NoError(t, err)
		require.Empty(t, undo.ResponseActivityID)

		requireNoFollow(t, db, remoteActor.ID, project.ID)
		requireActivityForObject(t, db, "Undo", followAPID)
		requireInboxItem(t, db, project.ID, "Undo", followAPID)

		localReply, err := commentService.CreateComment(ctx, localTicket.ID, owner.ID, "Thanks for the review.")
		require.NoError(t, err)
		requireDeliveryForObject(t, db, "Create", localReply.APID, remoteActor.InboxURL)

		ticketAfterUndo, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
			Title:       "No delivery after undo",
			Description: "Remote follower opted out",
			Priority:    "medium",
			Type:        "task",
		}, project.ID, owner.ID)
		require.NoError(t, err)
		requireNoDeliveryForObject(t, db, "Create", ticketAfterUndo.APID, remoteActor.InboxURL)
	})

	t.Run("outbox delivery ledger tracks attempts", func(t *testing.T) {
		var activityID string
		require.NoError(t, db.GetContext(ctx, &activityID, `
			SELECT id::text
			FROM ap_activities
			WHERE activity_type = 'Create' AND object_ap_id = $1
			LIMIT 1
		`, project.APID))

		repo := delivery.NewRepository(db)
		created, isNew, err := repo.Create(ctx, activityID, "https://remote.example/users/bot/inbox", 3)
		require.NoError(t, err)
		assert.True(t, isNew)
		assert.Equal(t, delivery.StatePending, created.State)

		duplicate, isNew, err := repo.Create(ctx, activityID, "https://remote.example/users/bot/inbox", 3)
		require.NoError(t, err)
		assert.False(t, isNew)
		assert.Equal(t, created.ID, duplicate.ID)

		started, err := repo.StartAttempt(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, delivery.StateProcessing, started.State)
		assert.Equal(t, 1, started.Attempts)
		require.NotNil(t, started.LastAttemptAt)

		nextAttempt := time.Now().UTC().Add(time.Minute)
		require.NoError(t, repo.MarkFailed(ctx, created.ID, "remote 503", delivery.FailureDetails{Kind: delivery.FailureKindHTTP, StatusCode: intPtr(http.StatusServiceUnavailable)}, &nextAttempt))

		retried, err := repo.StartAttempt(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, 2, retried.Attempts)

		require.NoError(t, repo.MarkDelivered(ctx, created.ID))
		_, err = repo.StartAttempt(ctx, created.ID)
		require.ErrorIs(t, err, delivery.ErrDeliveryDone)

		finalDelivery, isNew, err := repo.Create(ctx, activityID, "https://remote.example/users/dead/inbox", 2)
		require.NoError(t, err)
		assert.True(t, isNew)
		require.NoError(t, repo.MarkFailed(ctx, finalDelivery.ID, "permanent failure", delivery.FailureDetails{Kind: delivery.FailureKindUnknown}, nil))

		var terminal struct {
			State           string        `db:"state"`
			LastError       string        `db:"last_error"`
			LastFailureKind string        `db:"last_failure_kind"`
			LastStatusCode  sql.NullInt64 `db:"last_status_code"`
			NextAttemptAt   sql.NullTime  `db:"next_attempt_at"`
		}
		require.NoError(t, db.GetContext(ctx, &terminal, `
			SELECT state, last_error, last_failure_kind, last_status_code, next_attempt_at
			FROM activity_deliveries
			WHERE id = $1
		`, finalDelivery.ID))
		assert.Equal(t, delivery.StateDead, terminal.State)
		assert.Equal(t, "permanent failure", terminal.LastError)
		assert.Equal(t, delivery.FailureKindUnknown, terminal.LastFailureKind)
		assert.False(t, terminal.LastStatusCode.Valid)
		assert.False(t, terminal.NextAttemptAt.Valid)

		_, err = repo.StartAttempt(ctx, finalDelivery.ID)
		require.ErrorIs(t, err, delivery.ErrDeliveryExhausted)

		viewer, err := userService.RegisterUser(ctx, "retry-viewer", "retry-viewer@example.test", "password123")
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `
			INSERT INTO project_members (user_id, project_id, role)
			VALUES ($1, $2, 'viewer')
		`, viewer.ID, project.ID)
		require.NoError(t, err)

		retryQueue := &recordingDeliveryQueue{}
		retryService := delivery.NewService(delivery.NewRecipientRepository(db), retryQueue)
		summary, err := retryService.GetProjectDeliverySummary(ctx, project.ID, owner.ID)
		require.NoError(t, err)
		assert.True(t, summary.CanRetry)
		assert.GreaterOrEqual(t, summary.Dead, 1)
		assert.GreaterOrEqual(t, summary.Retryable, 1)

		deadDeliveries, err := retryService.ListProjectDeliveriesWithOptions(ctx, project.ID, owner.ID, delivery.ProjectDeliveryListOptions{
			State: delivery.StateDead,
			Limit: 1,
		})
		require.NoError(t, err)
		require.Len(t, deadDeliveries, 1)
		assert.Equal(t, finalDelivery.ID, deadDeliveries[0].ID)
		assert.True(t, deadDeliveries[0].CanRetry)

		viewerDeadDeliveries, err := retryService.ListProjectDeliveriesWithOptions(ctx, project.ID, viewer.ID, delivery.ProjectDeliveryListOptions{
			State: delivery.StateDead,
			Limit: 1,
		})
		require.NoError(t, err)
		require.Len(t, viewerDeadDeliveries, 1)
		assert.False(t, viewerDeadDeliveries[0].CanRetry)

		requeued, err := retryService.RetryProjectDelivery(ctx, project.ID, owner.ID, finalDelivery.ID)
		require.NoError(t, err)
		assert.Equal(t, delivery.StatePending, requeued.State)
		assert.Zero(t, requeued.Attempts)
		assert.Equal(t, delivery.DefaultMaxRetry, requeued.MaxAttempts)
		assert.Nil(t, requeued.NextAttemptAt)
		assert.Nil(t, requeued.LastError)
		assert.Equal(t, finalDelivery.ID, retryQueue.deliveryID)
		assert.Equal(t, delivery.DefaultMaxRetry, retryQueue.maxAttempts)

		_, err = retryService.RetryProjectDelivery(ctx, project.ID, viewer.ID, finalDelivery.ID)
		require.ErrorIs(t, err, delivery.ErrDeliveryRetryDenied)
	})
}

func TestFederationDomainBlockModeration(t *testing.T) {
	db := openIntegrationDB(t)
	resetIntegrationDB(t, db)

	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	userService := user.NewService(user.NewRepository(db, cfg), []byte("integration-secret"), cfg)
	moderationService := apmoderation.NewService(apmoderation.NewRepository(db))

	admin, err := userService.RegisterUser(ctx, "domain-admin", "domain-admin@example.test", "password123")
	require.NoError(t, err)
	worker, err := userService.RegisterUser(ctx, "domain-worker", "domain-worker@example.test", "password123")
	require.NoError(t, err)

	_, err = moderationService.BlockDomain(ctx, worker.ID, "remote.example", "spam")
	require.ErrorIs(t, err, apmoderation.ErrAdminRequired)
	requireRowCount(t, db, "federation_domain_blocks", 0)

	_, err = db.ExecContext(ctx, `UPDATE users SET role = 'admin' WHERE id = $1`, admin.ID)
	require.NoError(t, err)

	block, err := moderationService.BlockDomain(ctx, admin.ID, "HTTPS://Remote.Example/users/alice", " spam source ")
	require.NoError(t, err)
	assert.Equal(t, "remote.example", block.Domain)
	assert.Equal(t, "spam source", block.Reason)
	require.NotNil(t, block.CreatedBy)
	assert.Equal(t, admin.ID, *block.CreatedBy)

	blocks, err := moderationService.ListDomainBlocks(ctx, admin.ID)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, "remote.example", blocks[0].Domain)

	require.NoError(t, moderationService.UnblockDomain(ctx, admin.ID, "Remote.Example"))
	requireRowCount(t, db, "federation_domain_blocks", 0)
}

func TestFederationModerationInspection(t *testing.T) {
	db := openIntegrationDB(t)
	resetIntegrationDB(t, db)

	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	userService := user.NewService(user.NewRepository(db, cfg), []byte("integration-secret"), cfg)
	projectService := project.NewService(project.NewRepository(db, cfg), cfg)
	ticketService := ticket.NewService(ticket.NewRepository(db, cfg), projectService, cfg)
	moderationService := apmoderation.NewService(apmoderation.NewRepository(db))

	admin, err := userService.RegisterUser(ctx, "ops-admin", "ops-admin@example.test", "password123")
	require.NoError(t, err)
	owner, err := userService.RegisterUser(ctx, "ops-owner", "ops-owner@example.test", "password123")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE users SET role = 'admin' WHERE id = $1`, admin.ID)
	require.NoError(t, err)

	remoteActor := createRemoteActorWithPublicKey(t, ctx, db, "https://ops-remote.example/users/reviewer", "ops-reviewer", "public key")
	require.NoError(t, remoteactor.NewRepository(db).RecordRemoteActorFetchFailure(ctx, remoteActor.APID, "actor document timed out"))

	actors, err := moderationService.ListRemoteActors(ctx, admin.ID, apmoderation.RemoteActorListOptions{FetchErrorOnly: true})
	require.NoError(t, err)
	require.Len(t, actors, 1)
	assert.Equal(t, remoteActor.APID, actors[0].APID)
	require.NotNil(t, actors[0].FetchError)
	assert.Equal(t, "actor document timed out", *actors[0].FetchError)
	require.NotNil(t, actors[0].FetchErrorAt)

	createdProject, err := projectService.CreateProject(ctx, "Ops Inspection Board", "", owner.ID)
	require.NoError(t, err)
	createdTicket, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
		Title:    "Inspect failed delivery",
		Priority: "high",
		Type:     "task",
	}, createdProject.ID, owner.ID)
	require.NoError(t, err)
	activityID := requireActivityIDForObject(t, db, createdTicket.APID)
	deliveryRepo := delivery.NewRepository(db)
	createdDelivery, isNew, err := deliveryRepo.Create(ctx, activityID, remoteActor.InboxURL, 3)
	require.NoError(t, err)
	require.True(t, isNew)
	_, err = deliveryRepo.StartAttempt(ctx, createdDelivery.ID)
	require.NoError(t, err)
	require.NoError(t, deliveryRepo.MarkFailed(ctx, createdDelivery.ID, "remote 503", delivery.FailureDetails{Kind: delivery.FailureKindHTTP, StatusCode: intPtr(http.StatusServiceUnavailable)}, nil))

	deliveries, err := moderationService.ListFederationDeliveries(ctx, admin.ID, apmoderation.FederationDeliveryListOptions{
		State:       delivery.StateDead,
		FailureKind: delivery.FailureKindHTTP,
	})
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, createdDelivery.ID, deliveries[0].ID)
	assert.Equal(t, delivery.StateDead, deliveries[0].State)
	assert.Equal(t, delivery.FailureKindHTTP, deliveries[0].LastFailureKind)
	require.NotNil(t, deliveries[0].LastStatusCode)
	assert.Equal(t, http.StatusServiceUnavailable, *deliveries[0].LastStatusCode)
	assert.True(t, deliveries[0].CanRetry)
}

func TestRemoteInboxRefreshesRotatedActorKey(t *testing.T) {
	db := openIntegrationDB(t)
	fx := newInboxIntegrationFixture(t, db)

	oldPublicKey, _, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)
	newPublicKey, newPrivateKey, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/rotating-key" {
			http.NotFound(w, r)
			return
		}
		doc := activitypub.ActorDocument(
			"Person",
			server.URL+"/users/rotating-key",
			"rotating-key",
			"Rotating Key",
			"Served after a key rotation.",
			newPublicKey,
		)
		rawDoc, err := activitypub.MarshalDocument(doc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/activity+json")
		_, _ = w.Write(rawDoc)
	}))
	defer server.Close()

	actorAPID := server.URL + "/users/rotating-key"
	remoteActor := createRemoteActorWithPublicKey(t, fx.Ctx, db, actorAPID, "rotating-key", oldPublicKey)
	requireActorPublicKey(t, db, remoteActor.PublicKeyID, oldPublicKey)

	activityAPID := server.URL + "/activities/follow-with-rotated-key"
	body := []byte(`{"id":"` + activityAPID + `","type":"Follow","actor":"` + actorAPID + `","object":"` + fx.Project.APID + `"}`)
	req := newInboxPostRequest(t, fx.Project.APID, body)
	signRequestWithKey(t, fx.Ctx, req, remoteActor.ID, &httpsig.ActorKey{
		ActorID:       remoteActor.ID,
		ActorAPID:     remoteActor.APID,
		KeyID:         remoteActor.PublicKeyID,
		Algorithm:     httpsig.AlgorithmRSAV15SHA256,
		PublicKeyPEM:  newPublicKey,
		PrivateKeyPEM: newPrivateKey,
	}, body)

	remoteActorService := remoteactor.NewService(
		remoteactor.NewRepository(db),
		remoteactor.WithHTTPClient(server.Client()),
	)
	receiver := remoteinbox.NewService(
		remoteinbox.NewRepository(db, fx.Cfg),
		httpsig.NewService(
			httpsig.NewRepository(db),
			httpsig.WithMissingKeyResolver(remoteActorService.ResolveKey),
			httpsig.WithKeyRefreshResolver(remoteActorService.RefreshKey),
		),
	)

	accepted, err := receiver.Receive(fx.Ctx, req, fx.Project.APID, body)

	require.NoError(t, err)
	require.Equal(t, activityAPID, accepted.ActivityAPID)
	require.Empty(t, accepted.ResponseActivityID)
	requireNoFollow(t, db, remoteActor.ID, fx.Project.ID)
	requireActivityForObject(t, db, "Follow", fx.Project.APID)
	requireActorPublicKey(t, db, remoteActor.PublicKeyID, newPublicKey)
}

func TestRemoteInboxHandlesInviteResponses(t *testing.T) {
	db := openIntegrationDB(t)
	resetIntegrationDB(t, db)

	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	userService := user.NewService(user.NewRepository(db, cfg), []byte("integration-secret"), cfg)
	projectService := project.NewService(project.NewRepository(db, cfg), cfg)
	receiver := remoteinbox.NewService(
		remoteinbox.NewRepository(db, cfg),
		httpsig.NewService(httpsig.NewRepository(db)),
	)

	owner, err := userService.RegisterUser(ctx, "owner", "owner@example.test", "password123")
	require.NoError(t, err)
	createdProject, err := projectService.CreateProject(ctx, "Invite Response Board", "", owner.ID)
	require.NoError(t, err)

	t.Run("accepts remote invite response idempotently", func(t *testing.T) {
		remoteActor, privateKey := createRemoteActor(t, ctx, db, "invite-accept")
		invite, err := projectService.AddMemberToProject(ctx, createdProject.ID, owner.ID, remoteActor.ID, "viewer")
		require.NoError(t, err)

		activityAPID := "https://remote.example/activities/accept-project-invite"
		body := []byte(`{"id":"` + activityAPID + `","type":"Accept","actor":"` + remoteActor.APID + `","object":"` + invite.APID + `","target":"` + createdProject.APID + `"}`)
		req := signedRemoteInboxRequest(t, ctx, createdProject.APID, remoteActor, privateKey, body)

		accepted, err := receiver.Receive(ctx, req, createdProject.APID, body)
		require.NoError(t, err)
		require.False(t, accepted.Duplicate)
		requireInviteStatus(t, db, invite.ID, "accepted")
		requireFollow(t, db, remoteActor.ID, createdProject.ID, "accepted")
		requireNoProjectMember(t, db, remoteActor.ID, createdProject.ID)
		requireActivityForObjectAndActor(t, db, "Accept", invite.APID, remoteActor.ID)
		requireInboxItem(t, db, createdProject.ID, "Accept", invite.APID)

		duplicate, err := receiver.Receive(ctx, req, createdProject.APID, body)
		require.NoError(t, err)
		require.True(t, duplicate.Duplicate)
		requireInboxActivityCount(t, db, activityAPID, 1)

		nonPendingAPID := "https://remote.example/activities/reject-accepted-invite"
		nonPendingBody := []byte(`{"id":"` + nonPendingAPID + `","type":"Reject","actor":"` + remoteActor.APID + `","object":"` + invite.APID + `","target":"` + createdProject.APID + `"}`)
		nonPendingReq := signedRemoteInboxRequest(t, ctx, createdProject.APID, remoteActor, privateKey, nonPendingBody)

		_, err = receiver.Receive(ctx, nonPendingReq, createdProject.APID, nonPendingBody)
		require.ErrorIs(t, err, remoteinbox.ErrInvalidActivity)
		requireInviteStatus(t, db, invite.ID, "accepted")
		requireNoActivityByAPID(t, db, nonPendingAPID)
		requireNoInboxActivity(t, db, nonPendingAPID)
	})

	t.Run("rejects remote invite response", func(t *testing.T) {
		remoteActor, privateKey := createRemoteActor(t, ctx, db, "invite-reject")
		invite, err := projectService.AddMemberToProject(ctx, createdProject.ID, owner.ID, remoteActor.ID, "viewer")
		require.NoError(t, err)

		activityAPID := "https://remote.example/activities/reject-project-invite"
		body := []byte(`{"id":"` + activityAPID + `","type":"Reject","actor":"` + remoteActor.APID + `","object":{"id":"` + invite.APID + `","type":"Invite","actor":"` + owner.APID + `","object":"` + createdProject.APID + `"},"target":"` + createdProject.APID + `"}`)
		req := signedRemoteInboxRequest(t, ctx, createdProject.APID, remoteActor, privateKey, body)

		accepted, err := receiver.Receive(ctx, req, createdProject.APID, body)
		require.NoError(t, err)
		require.False(t, accepted.Duplicate)
		requireInviteStatus(t, db, invite.ID, "rejected")
		requireNoFollow(t, db, remoteActor.ID, createdProject.ID)
		requireActivityForObjectAndActor(t, db, "Reject", invite.APID, remoteActor.ID)
		requireInboxItem(t, db, createdProject.ID, "Reject", invite.APID)
	})

	t.Run("rejects invite response from wrong actor", func(t *testing.T) {
		invitee, _ := createRemoteActor(t, ctx, db, "invite-target")
		mallory, privateKey := createRemoteActor(t, ctx, db, "invite-mallory")
		invite, err := projectService.AddMemberToProject(ctx, createdProject.ID, owner.ID, invitee.ID, "viewer")
		require.NoError(t, err)

		activityAPID := "https://remote.example/activities/wrong-actor-invite-accept"
		body := []byte(`{"id":"` + activityAPID + `","type":"Accept","actor":"` + mallory.APID + `","object":"` + invite.APID + `","target":"` + createdProject.APID + `"}`)
		req := signedRemoteInboxRequest(t, ctx, createdProject.APID, mallory, privateKey, body)

		_, err = receiver.Receive(ctx, req, createdProject.APID, body)
		require.ErrorIs(t, err, remoteinbox.ErrForbiddenActor)
		requireInviteStatus(t, db, invite.ID, "pending")
		requireNoFollow(t, db, mallory.ID, createdProject.ID)
		requireNoActivityByAPID(t, db, activityAPID)
		requireNoInboxActivity(t, db, activityAPID)
	})

	t.Run("rejects invite response sent to wrong project", func(t *testing.T) {
		remoteActor, privateKey := createRemoteActor(t, ctx, db, "invite-wrong-project")
		invite, err := projectService.AddMemberToProject(ctx, createdProject.ID, owner.ID, remoteActor.ID, "viewer")
		require.NoError(t, err)
		otherProject, err := projectService.CreateProject(ctx, "Other Invite Board", "", owner.ID)
		require.NoError(t, err)

		activityAPID := "https://remote.example/activities/wrong-project-invite-accept"
		body := []byte(`{"id":"` + activityAPID + `","type":"Accept","actor":"` + remoteActor.APID + `","object":"` + invite.APID + `","target":"` + otherProject.APID + `"}`)
		req := signedRemoteInboxRequest(t, ctx, otherProject.APID, remoteActor, privateKey, body)

		_, err = receiver.Receive(ctx, req, otherProject.APID, body)
		require.ErrorIs(t, err, remoteinbox.ErrInvalidActivity)
		requireInviteStatus(t, db, invite.ID, "pending")
		requireNoFollow(t, db, remoteActor.ID, otherProject.ID)
		requireNoActivityByAPID(t, db, activityAPID)
		requireNoInboxActivity(t, db, activityAPID)
	})
}

func TestRemoteInboxRejectsUnsafeInboundActivities(t *testing.T) {
	db := openIntegrationDB(t)

	t.Run("rejects signature actor mismatch without persisting", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		remoteActor, privateKey := createRemoteActor(t, fx.Ctx, db, "mismatch-signer")

		activityAPID := "https://remote.example/activities/mismatch-follow"
		body := []byte(`{"id":"` + activityAPID + `","type":"Follow","actor":"https://remote.example/users/mallory","object":"` + fx.Project.APID + `"}`)
		req := signedRemoteInboxRequest(t, fx.Ctx, fx.Project.APID, remoteActor, privateKey, body)

		_, err := fx.Receiver.Receive(fx.Ctx, req, fx.Project.APID, body)

		require.ErrorIs(t, err, remoteinbox.ErrForbiddenActor)
		requireNoActivityByAPID(t, db, activityAPID)
		requireNoInboxActivity(t, db, activityAPID)
		requireNoFollow(t, db, remoteActor.ID, fx.Project.ID)
	})

	t.Run("rejects missing body digest without persisting", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		remoteActor, privateKey := createRemoteActor(t, fx.Ctx, db, "missing-digest")

		activityAPID := "https://remote.example/activities/missing-digest-follow"
		body := []byte(`{"id":"` + activityAPID + `","type":"Follow","actor":"` + remoteActor.APID + `","object":"` + fx.Project.APID + `"}`)
		req := newInboxPostRequest(t, fx.Project.APID, body)
		signRemoteInboxRequest(t, fx.Ctx, req, remoteActor, privateKey, nil)

		_, err := fx.Receiver.Receive(fx.Ctx, req, fx.Project.APID, body)

		require.ErrorIs(t, err, remoteinbox.ErrUnauthorized)
		requireNoActivityByAPID(t, db, activityAPID)
		requireNoInboxActivity(t, db, activityAPID)
		requireNoFollow(t, db, remoteActor.ID, fx.Project.ID)
	})

	t.Run("rejects stale signed date without persisting", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		remoteActor, privateKey := createRemoteActor(t, fx.Ctx, db, "stale-date")

		activityAPID := "https://remote.example/activities/stale-date-follow"
		body := []byte(`{"id":"` + activityAPID + `","type":"Follow","actor":"` + remoteActor.APID + `","object":"` + fx.Project.APID + `"}`)
		req := newInboxPostRequest(t, fx.Project.APID, body)
		req.Header.Set("Date", time.Now().UTC().Add(-10*time.Minute).Format(http.TimeFormat))
		signRemoteInboxRequest(t, fx.Ctx, req, remoteActor, privateKey, body)

		_, err := fx.Receiver.Receive(fx.Ctx, req, fx.Project.APID, body)

		require.ErrorIs(t, err, remoteinbox.ErrUnauthorized)
		requireNoActivityByAPID(t, db, activityAPID)
		requireNoInboxActivity(t, db, activityAPID)
		requireNoFollow(t, db, remoteActor.ID, fx.Project.ID)
	})

	t.Run("rejects resolved actor key mismatch without caching actor", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		publicKey, privateKey, err := activitypub.GenerateRSAKeyPair()
		require.NoError(t, err)

		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/users/key-mismatch" {
				http.NotFound(w, r)
				return
			}
			doc := activitypub.ActorDocument(
				"Person",
				server.URL+"/users/key-mismatch",
				"key-mismatch",
				"Key Mismatch",
				"Served with a different key id.",
				publicKey,
			)
			rawDoc, err := activitypub.MarshalDocument(doc)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/activity+json")
			_, _ = w.Write(rawDoc)
		}))
		defer server.Close()

		actorAPID := server.URL + "/users/key-mismatch"
		keyID := actorAPID + "#rotated-key"
		activityAPID := server.URL + "/activities/key-mismatch-follow"
		body := []byte(`{"id":"` + activityAPID + `","type":"Follow","actor":"` + actorAPID + `","object":"` + fx.Project.APID + `"}`)
		req := newInboxPostRequest(t, fx.Project.APID, body)
		signRequestWithKey(t, fx.Ctx, req, "remote-key-mismatch-before-cache", &httpsig.ActorKey{
			ActorID:       "remote-key-mismatch-before-cache",
			ActorAPID:     actorAPID,
			KeyID:         keyID,
			Algorithm:     httpsig.AlgorithmRSAV15SHA256,
			PublicKeyPEM:  publicKey,
			PrivateKeyPEM: privateKey,
		}, body)

		remoteActorService := remoteactor.NewService(
			remoteactor.NewRepository(db),
			remoteactor.WithHTTPClient(server.Client()),
		)
		receiver := remoteinbox.NewService(
			remoteinbox.NewRepository(db, fx.Cfg),
			httpsig.NewService(
				httpsig.NewRepository(db),
				httpsig.WithMissingKeyResolver(remoteActorService.ResolveKey),
			),
		)

		_, err = receiver.Receive(fx.Ctx, req, fx.Project.APID, body)

		require.ErrorIs(t, err, remoteinbox.ErrUnauthorized)
		requireNoActorByAPID(t, db, actorAPID)
		requireNoActivityByAPID(t, db, activityAPID)
		requireNoInboxActivity(t, db, activityAPID)
	})

	t.Run("rejects known actor refresh with changed key id", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		oldPublicKey, _, err := activitypub.GenerateRSAKeyPair()
		require.NoError(t, err)
		newPublicKey, newPrivateKey, err := activitypub.GenerateRSAKeyPair()
		require.NoError(t, err)

		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/users/changed-key-id" {
				http.NotFound(w, r)
				return
			}
			doc := activitypub.ActorDocument(
				"Person",
				server.URL+"/users/changed-key-id",
				"changed-key-id",
				"Changed Key ID",
				"Served with an unexpected key id.",
				newPublicKey,
			)
			doc["publicKey"].(map[string]any)["id"] = server.URL + "/users/changed-key-id#rotated-key"
			rawDoc, err := activitypub.MarshalDocument(doc)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/activity+json")
			_, _ = w.Write(rawDoc)
		}))
		defer server.Close()

		actorAPID := server.URL + "/users/changed-key-id"
		remoteActor := createRemoteActorWithPublicKey(t, fx.Ctx, db, actorAPID, "changed-key-id", oldPublicKey)

		activityAPID := server.URL + "/activities/reject-changed-key-id"
		body := []byte(`{"id":"` + activityAPID + `","type":"Follow","actor":"` + actorAPID + `","object":"` + fx.Project.APID + `"}`)
		req := newInboxPostRequest(t, fx.Project.APID, body)
		signRequestWithKey(t, fx.Ctx, req, remoteActor.ID, &httpsig.ActorKey{
			ActorID:       remoteActor.ID,
			ActorAPID:     remoteActor.APID,
			KeyID:         remoteActor.PublicKeyID,
			Algorithm:     httpsig.AlgorithmRSAV15SHA256,
			PublicKeyPEM:  newPublicKey,
			PrivateKeyPEM: newPrivateKey,
		}, body)

		remoteActorService := remoteactor.NewService(
			remoteactor.NewRepository(db),
			remoteactor.WithHTTPClient(server.Client()),
		)
		receiver := remoteinbox.NewService(
			remoteinbox.NewRepository(db, fx.Cfg),
			httpsig.NewService(
				httpsig.NewRepository(db),
				httpsig.WithKeyRefreshResolver(remoteActorService.RefreshKey),
			),
		)

		_, err = receiver.Receive(fx.Ctx, req, fx.Project.APID, body)

		require.ErrorIs(t, err, remoteinbox.ErrUnauthorized)
		requireActorPublicKey(t, db, remoteActor.PublicKeyID, oldPublicKey)
		requireNoActivityByAPID(t, db, activityAPID)
		requireNoInboxActivity(t, db, activityAPID)
		requireNoFollow(t, db, remoteActor.ID, fx.Project.ID)
	})

	t.Run("rejects ticket mutations from non followers without projections", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		remoteActor, privateKey := createRemoteActor(t, fx.Ctx, db, "not-a-follower")

		cases := []struct {
			name         string
			activityAPID string
			ticketAPID   string
			body         []byte
		}{
			{
				name:         "create ticket",
				activityAPID: "https://remote.example/activities/non-follower-create-ticket",
				ticketAPID:   "https://remote.example/tickets/non-follower-create",
				body:         []byte(`{"id":"https://remote.example/activities/non-follower-create-ticket","type":"Create","actor":"` + remoteActor.APID + `","object":{"id":"https://remote.example/tickets/non-follower-create","type":"forge:Ticket","attributedTo":"` + remoteActor.APID + `","context":"` + fx.Project.APID + `","name":"Blocked remote ticket","content":"Should not land.","forge:priority":"medium","forge:ticketType":"task","forge:isResolved":false}}`),
			},
			{
				name:         "update ticket",
				activityAPID: "https://remote.example/activities/non-follower-update-ticket",
				ticketAPID:   "https://remote.example/tickets/non-follower-update",
				body:         []byte(`{"id":"https://remote.example/activities/non-follower-update-ticket","type":"Update","actor":"` + remoteActor.APID + `","target":"` + fx.Project.APID + `","object":{"id":"https://remote.example/tickets/non-follower-update","type":"forge:Ticket","attributedTo":"` + remoteActor.APID + `","context":"` + fx.Project.APID + `","name":"Blocked update"}}`),
			},
			{
				name:         "delete ticket",
				activityAPID: "https://remote.example/activities/non-follower-delete-ticket",
				ticketAPID:   "https://remote.example/tickets/non-follower-delete",
				body:         []byte(`{"id":"https://remote.example/activities/non-follower-delete-ticket","type":"Delete","actor":"` + remoteActor.APID + `","object":"https://remote.example/tickets/non-follower-delete","target":"` + fx.Project.APID + `"}`),
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				req := signedRemoteInboxRequest(t, fx.Ctx, fx.Project.APID, remoteActor, privateKey, tc.body)

				_, err := fx.Receiver.Receive(fx.Ctx, req, fx.Project.APID, tc.body)

				require.ErrorIs(t, err, remoteinbox.ErrForbiddenActor)
				requireNoActivityByAPID(t, db, tc.activityAPID)
				requireNoInboxActivity(t, db, tc.activityAPID)
				requireNoTicketByAPID(t, db, tc.ticketAPID)
				requireNoObjectByAPID(t, db, tc.ticketAPID)
			})
		}
	})

	t.Run("rejects duplicate activity id from another actor", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		firstActor, firstPrivateKey := createRemoteActor(t, fx.Ctx, db, "duplicate-first")
		secondActor, secondPrivateKey := createRemoteActor(t, fx.Ctx, db, "duplicate-second")

		activityAPID := "https://remote.example/activities/reused-activity-id"
		firstObjectAPID := "https://remote.example/objects/reused-first"
		firstBody := []byte(`{"id":"` + activityAPID + `","type":"Create","actor":"` + firstActor.APID + `","object":"` + firstObjectAPID + `"}`)
		firstReq := signedRemoteInboxRequest(t, fx.Ctx, fx.Project.APID, firstActor, firstPrivateKey, firstBody)

		accepted, err := fx.Receiver.Receive(fx.Ctx, firstReq, fx.Project.APID, firstBody)
		require.NoError(t, err)
		require.Equal(t, activityAPID, accepted.ActivityAPID)

		secondBody := []byte(`{"id":"` + activityAPID + `","type":"Create","actor":"` + secondActor.APID + `","object":"https://remote.example/objects/reused-second"}`)
		secondReq := signedRemoteInboxRequest(t, fx.Ctx, fx.Project.APID, secondActor, secondPrivateKey, secondBody)

		_, err = fx.Receiver.Receive(fx.Ctx, secondReq, fx.Project.APID, secondBody)

		require.ErrorIs(t, err, remoteinbox.ErrActivityConflict)
		requireActivityActor(t, db, activityAPID, firstActor.ID)
		requireInboxActivityCount(t, db, activityAPID, 1)
		requireInboxItem(t, db, fx.Project.ID, "Create", firstObjectAPID)
	})

	t.Run("rejects malformed project activities without writes", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		remoteActor, privateKey := createRemoteActor(t, fx.Ctx, db, "malformed")

		cases := []struct {
			name         string
			activityAPID string
			body         []byte
		}{
			{
				name:         "follow wrong object",
				activityAPID: "https://remote.example/activities/malformed-follow",
				body:         []byte(`{"id":"https://remote.example/activities/malformed-follow","type":"Follow","actor":"` + remoteActor.APID + `","object":"https://remote.example/projects/other"}`),
			},
			{
				name:         "blank note content",
				activityAPID: "https://remote.example/activities/malformed-note",
				body:         []byte(`{"id":"https://remote.example/activities/malformed-note","type":"Create","actor":"` + remoteActor.APID + `","object":{"id":"https://remote.example/notes/malformed","type":"Note","attributedTo":"` + remoteActor.APID + `","inReplyTo":"http://localhost:8080/tickets/missing","content":"   "}}`),
			},
			{
				name:         "invalid ticket priority",
				activityAPID: "https://remote.example/activities/malformed-ticket",
				body:         []byte(`{"id":"https://remote.example/activities/malformed-ticket","type":"Create","actor":"` + remoteActor.APID + `","object":{"id":"https://remote.example/tickets/malformed","type":"forge:Ticket","attributedTo":"` + remoteActor.APID + `","context":"` + fx.Project.APID + `","name":"Malformed ticket","forge:priority":"eventually"}}`),
			},
			{
				name:         "add without target",
				activityAPID: "https://remote.example/activities/malformed-add",
				body:         []byte(`{"id":"https://remote.example/activities/malformed-add","type":"Add","actor":"` + remoteActor.APID + `","object":"http://localhost:8080/users/owner"}`),
			},
			{
				name:         "delete project object",
				activityAPID: "https://remote.example/activities/malformed-delete",
				body:         []byte(`{"id":"https://remote.example/activities/malformed-delete","type":"Delete","actor":"` + remoteActor.APID + `","object":"` + fx.Project.APID + `"}`),
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				req := signedRemoteInboxRequest(t, fx.Ctx, fx.Project.APID, remoteActor, privateKey, tc.body)

				_, err := fx.Receiver.Receive(fx.Ctx, req, fx.Project.APID, tc.body)

				require.ErrorIs(t, err, remoteinbox.ErrInvalidActivity)
				requireNoActivityByAPID(t, db, tc.activityAPID)
				requireNoInboxActivity(t, db, tc.activityAPID)
			})
		}
	})
}

type inboxIntegrationFixture struct {
	Ctx      context.Context
	Cfg      activitypub.Config
	Project  *project.Project
	Receiver *remoteinbox.Service
}

func newInboxIntegrationFixture(t *testing.T, db *sqlx.DB) inboxIntegrationFixture {
	t.Helper()

	resetIntegrationDB(t, db)

	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	userService := user.NewService(user.NewRepository(db, cfg), []byte("integration-secret"), cfg)
	projectService := project.NewService(project.NewRepository(db, cfg), cfg)

	owner, err := userService.RegisterUser(ctx, "owner", "owner@example.test", "password123")
	require.NoError(t, err)
	createdProject, err := projectService.CreateProject(ctx, "Inbox Rejection Board", "", owner.ID)
	require.NoError(t, err)

	return inboxIntegrationFixture{
		Ctx:     ctx,
		Cfg:     cfg,
		Project: createdProject,
		Receiver: remoteinbox.NewService(
			remoteinbox.NewRepository(db, cfg),
			httpsig.NewService(httpsig.NewRepository(db)),
		),
	}
}

func createRemoteActor(t *testing.T, ctx context.Context, db *sqlx.DB, username string) (*remoteactor.Actor, string) {
	t.Helper()

	publicKey, privateKey, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	actorAPID := "https://remote.example/users/" + username
	actor := createRemoteActorWithPublicKey(t, ctx, db, actorAPID, username, publicKey)
	return actor, privateKey
}

func createRemoteActorWithPublicKey(t *testing.T, ctx context.Context, db *sqlx.DB, actorAPID, username, publicKey string) *remoteactor.Actor {
	t.Helper()

	doc := activitypub.ActorDocument("Person", actorAPID, username, username, "", publicKey)
	rawDoc, err := activitypub.MarshalDocument(doc)
	require.NoError(t, err)

	actor := &remoteactor.Actor{
		APID:              actorAPID,
		Type:              "Person",
		PreferredUsername: username,
		Handle:            username + "@remote.example",
		Name:              username,
		Summary:           "",
		InboxURL:          activitypub.Inbox(actorAPID),
		OutboxURL:         activitypub.Outbox(actorAPID),
		PublicKeyID:       activitypub.KeyID(actorAPID),
		PublicKeyPEM:      publicKey,
		Document:          rawDoc,
	}
	require.NoError(t, remoteactor.NewRepository(db).UpsertRemoteActor(ctx, actor))
	return actor
}

func newInboxPostRequest(t *testing.T, targetAPID string, body []byte) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, targetAPID+"/inbox", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/activity+json")
	return req
}

func signedRemoteInboxRequest(t *testing.T, ctx context.Context, targetAPID string, actor *remoteactor.Actor, privateKey string, body []byte) *http.Request {
	t.Helper()

	req := newInboxPostRequest(t, targetAPID, body)
	signRemoteInboxRequest(t, ctx, req, actor, privateKey, body)
	return req
}

func signRemoteInboxRequest(t *testing.T, ctx context.Context, req *http.Request, actor *remoteactor.Actor, privateKey string, body []byte) {
	t.Helper()

	signRequestWithKey(t, ctx, req, actor.ID, &httpsig.ActorKey{
		ActorID:       actor.ID,
		ActorAPID:     actor.APID,
		KeyID:         actor.PublicKeyID,
		Algorithm:     httpsig.AlgorithmRSAV15SHA256,
		PublicKeyPEM:  actor.PublicKeyPEM,
		PrivateKeyPEM: privateKey,
	}, body)
}

func signRequestWithKey(t *testing.T, ctx context.Context, req *http.Request, actorID string, key *httpsig.ActorKey, body []byte) {
	t.Helper()

	signer := httpsig.NewService(singleKeyRepo{key: key})
	require.NoError(t, signer.SignRequest(ctx, actorID, req, body))
}

func requireActivityPubReadStatus(t *testing.T, e *echo.Echo, path, authorization string, prepare func(*http.Request), expectedStatus int) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authorization != "" {
		req.Header.Set(echo.HeaderAuthorization, authorization)
	}
	if prepare != nil {
		prepare(req)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, expectedStatus, rec.Code, rec.Body.String())
	if expectedStatus == http.StatusOK {
		require.Contains(t, rec.Header().Get(echo.HeaderContentType), activitypub.ActivityJSONMediaType)
	}
}

type integrationSignatureActorVerifier struct {
	service *httpsig.Service
}

func (v integrationSignatureActorVerifier) VerifyActorID(ctx context.Context, req *http.Request) (string, error) {
	verified, err := v.service.VerifyRequest(ctx, req, nil)
	if err != nil {
		return "", err
	}
	return verified.ActorID, nil
}

type singleKeyRepo struct {
	key *httpsig.ActorKey
}

type recordingDeliveryQueue struct {
	deliveryID  string
	maxAttempts int
	err         error
}

func (q *recordingDeliveryQueue) Enqueue(ctx context.Context, deliveryID string, maxAttempts int) error {
	q.deliveryID = deliveryID
	q.maxAttempts = maxAttempts
	return q.err
}

func (q *recordingDeliveryQueue) Close() error {
	return nil
}

func (r singleKeyRepo) ActivePrivateKey(ctx context.Context, actorID string) (*httpsig.ActorKey, error) {
	return r.key, nil
}

func (r singleKeyRepo) PublicKeyByKeyID(ctx context.Context, keyID string) (*httpsig.ActorKey, error) {
	return r.key, nil
}

func openIntegrationDB(t *testing.T) *sqlx.DB {
	t.Helper()

	source := os.Getenv("TEST_DB_SOURCE")
	if source == "" {
		t.Skip("set TEST_DB_SOURCE to run integration tests against a migrated PostgreSQL database")
	}

	return newSchemaIntegrationDB(t, context.Background(), source, integrationTestSchemaName(t.Name()))
}

func integrationTestSchemaName(name string) string {
	var schema strings.Builder
	schema.WriteString("test_")
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z':
			schema.WriteRune(r)
		case r >= '0' && r <= '9':
			schema.WriteRune(r)
		default:
			schema.WriteByte('_')
		}
		if schema.Len() >= 54 {
			break
		}
	}
	return strings.TrimRight(schema.String(), "_")
}

func resetIntegrationDB(t *testing.T, db *sqlx.DB) {
	t.Helper()

	_, err := db.Exec(`
		TRUNCATE TABLE
			actor_outbox_items,
			actor_inbox_items,
			federation_domain_blocks,
			project_invites,
			activity_deliveries,
			ap_activities,
			ap_objects,
			comments,
			ticket_assignees,
			ticket_links,
			tickets,
			actor_follows,
			project_members,
			projects,
			users,
			actor_keys,
			actors
		CASCADE
	`)
	require.NoError(t, err)
}

func requireRowCount(t *testing.T, db *sqlx.DB, tableName string, expected int) {
	t.Helper()

	var actual int
	err := db.Get(&actual, "SELECT count(*) FROM "+tableName)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func findTicketByAPID(tickets []ticket.Ticket, apID string) *ticket.Ticket {
	for i := range tickets {
		if tickets[i].APID == apID {
			return &tickets[i]
		}
	}
	return nil
}

func projectUpdateNameRequest(name *string) project.UpdateProjectRequest {
	return project.UpdateProjectRequest{Name: name}
}

func requireObjectType(t *testing.T, db *sqlx.DB, apID, expectedType string) {
	t.Helper()

	var objectType string
	err := db.Get(&objectType, `SELECT object_type FROM ap_objects WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Equal(t, expectedType, objectType)
}

func requireCommentContent(t *testing.T, db *sqlx.DB, apID, expectedContent string) {
	t.Helper()

	var content string
	err := db.Get(&content, `SELECT content FROM comments WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Equal(t, expectedContent, content)
}

func requireTicketObjectName(t *testing.T, db *sqlx.DB, apID, expectedName string) {
	t.Helper()

	var name string
	err := db.Get(&name, `SELECT document->>'name' FROM ap_objects WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Equal(t, expectedName, name)
}

func requireProjectObjectFields(t *testing.T, db *sqlx.DB, apID, expectedName, expectedSummary string) {
	t.Helper()

	var stored struct {
		Name      string `db:"name"`
		Summary   string `db:"summary"`
		IsDeleted bool   `db:"is_deleted"`
	}
	err := db.Get(&stored, `
		SELECT
			document->>'name' AS name,
			document->>'summary' AS summary,
			is_deleted
		FROM ap_objects
		WHERE ap_id = $1
	`, apID)
	require.NoError(t, err)
	require.Equal(t, expectedName, stored.Name)
	require.Equal(t, expectedSummary, stored.Summary)
	require.False(t, stored.IsDeleted)
}

func requireObjectDeleted(t *testing.T, db *sqlx.DB, apID string) {
	t.Helper()

	var isDeleted bool
	err := db.Get(&isDeleted, `SELECT is_deleted FROM ap_objects WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.True(t, isDeleted)
}

func requireObjectTombstone(t *testing.T, db *sqlx.DB, apID, formerType string) {
	t.Helper()

	var stored struct {
		ObjectType string `db:"object_type"`
		Type       string `db:"type"`
		FormerType string `db:"former_type"`
		HasContent bool   `db:"has_content"`
		HasName    bool   `db:"has_name"`
	}
	err := db.Get(&stored, `
		SELECT
			object_type,
			document->>'type' AS type,
			document->>'formerType' AS former_type,
			document ? 'content' AS has_content,
			document ? 'name' AS has_name
		FROM ap_objects
		WHERE ap_id = $1
	`, apID)
	require.NoError(t, err)
	require.Equal(t, "Tombstone", stored.ObjectType)
	require.Equal(t, "Tombstone", stored.Type)
	require.Equal(t, formerType, stored.FormerType)
	require.False(t, stored.HasContent)
	require.False(t, stored.HasName)
}

func requireNoTicketByAPID(t *testing.T, db *sqlx.DB, apID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `SELECT count(*) FROM tickets WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireNoObjectByAPID(t *testing.T, db *sqlx.DB, apID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `SELECT count(*) FROM ap_objects WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireNoActorByAPID(t *testing.T, db *sqlx.DB, apID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `SELECT count(*) FROM actors WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireActorPublicKey(t *testing.T, db *sqlx.DB, keyID, expectedPublicKey string) {
	t.Helper()

	var publicKey string
	err := db.Get(&publicKey, `SELECT public_key_pem FROM actor_keys WHERE key_id = $1 AND active = true`, keyID)
	require.NoError(t, err)
	require.Equal(t, expectedPublicKey, publicKey)
}

func requireProjectRole(t *testing.T, db *sqlx.DB, userID, projectID, expectedRole string) {
	t.Helper()

	var role string
	err := db.Get(&role, `
		SELECT role
		FROM project_members
		WHERE user_id = $1 AND project_id = $2
	`, userID, projectID)
	require.NoError(t, err)
	require.Equal(t, expectedRole, role)
}

func requireNoProjectMember(t *testing.T, db *sqlx.DB, userID, projectID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM project_members
		WHERE user_id = $1 AND project_id = $2
	`, userID, projectID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireNoProjectByID(t *testing.T, db *sqlx.DB, projectID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `SELECT count(*) FROM projects WHERE id = $1`, projectID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireInviteStatus(t *testing.T, db *sqlx.DB, inviteID, expectedStatus string) {
	t.Helper()

	var status string
	err := db.Get(&status, `SELECT status FROM project_invites WHERE id = $1`, inviteID)
	require.NoError(t, err)
	require.Equal(t, expectedStatus, status)
}

func requireFollow(t *testing.T, db *sqlx.DB, followerID, followedID, expectedState string) {
	t.Helper()

	var state string
	err := db.Get(&state, `
		SELECT state
		FROM actor_follows
		WHERE follower_actor_id = $1 AND followed_actor_id = $2
	`, followerID, followedID)
	require.NoError(t, err)
	require.Equal(t, expectedState, state)
}

func requireNoFollow(t *testing.T, db *sqlx.DB, followerID, followedID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM actor_follows
		WHERE follower_actor_id = $1 AND followed_actor_id = $2
	`, followerID, followedID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireActivityType(t *testing.T, db *sqlx.DB, apID, expectedType string) {
	t.Helper()

	var activityType string
	err := db.Get(&activityType, `SELECT activity_type FROM ap_activities WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Equal(t, expectedType, activityType)
}

func requireActivityActor(t *testing.T, db *sqlx.DB, apID, expectedActorID string) {
	t.Helper()

	var actorID string
	err := db.Get(&actorID, `SELECT actor_id::text FROM ap_activities WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Equal(t, expectedActorID, actorID)
}

func activityCount(t *testing.T, db *sqlx.DB) int {
	t.Helper()

	var count int
	err := db.Get(&count, `SELECT count(*) FROM ap_activities`)
	require.NoError(t, err)
	return count
}

func requireActivityCount(t *testing.T, db *sqlx.DB, expected int) {
	t.Helper()

	require.Equal(t, expected, activityCount(t, db))
}

func requireNoActivityByAPID(t *testing.T, db *sqlx.DB, apID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `SELECT count(*) FROM ap_activities WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireActivityForObject(t *testing.T, db *sqlx.DB, activityType, objectAPID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM ap_activities
		WHERE activity_type = $1 AND object_ap_id = $2
	`, activityType, objectAPID)
	require.NoError(t, err)
	require.Greater(t, count, 0)
}

func requireActivityAPIDForObject(t *testing.T, db *sqlx.DB, activityType, objectAPID string) string {
	t.Helper()

	var apID string
	err := db.Get(&apID, `
		SELECT ap_id
		FROM ap_activities
		WHERE activity_type = $1 AND object_ap_id = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, activityType, objectAPID)
	require.NoError(t, err)
	return apID
}

func requireActivityForObjectAndActor(t *testing.T, db *sqlx.DB, activityType, objectAPID, actorID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM ap_activities
		WHERE activity_type = $1
			AND object_ap_id = $2
			AND actor_id = $3
	`, activityType, objectAPID, actorID)
	require.NoError(t, err)
	require.Greater(t, count, 0)
}

func requireActivityForObjectAndTarget(t *testing.T, db *sqlx.DB, activityType, objectAPID, targetAPID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM ap_activities
		WHERE activity_type = $1 AND object_ap_id = $2 AND target_ap_id = $3
	`, activityType, objectAPID, targetAPID)
	require.NoError(t, err)
	require.Greater(t, count, 0)
}

func requireDeliveryForObject(t *testing.T, db *sqlx.DB, activityType, objectAPID, inboxURL string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM activity_deliveries delivery
		JOIN ap_activities activity ON activity.id = delivery.activity_id
		WHERE activity.activity_type = $1
			AND activity.object_ap_id = $2
			AND delivery.target_inbox_url = $3
	`, activityType, objectAPID, inboxURL)
	require.NoError(t, err)
	require.Greater(t, count, 0)
}

func requireDeliveryForObjectFromActor(t *testing.T, db *sqlx.DB, activityType, objectAPID, inboxURL, actorID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM activity_deliveries delivery
		JOIN ap_activities activity ON activity.id = delivery.activity_id
		WHERE activity.activity_type = $1
			AND activity.object_ap_id = $2
			AND delivery.target_inbox_url = $3
			AND delivery.actor_id = $4
	`, activityType, objectAPID, inboxURL, actorID)
	require.NoError(t, err)
	require.Greater(t, count, 0)
}

func requireDeliveryCountForObject(t *testing.T, db *sqlx.DB, activityType, objectAPID, inboxURL string, expected int) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM activity_deliveries delivery
		JOIN ap_activities activity ON activity.id = delivery.activity_id
		WHERE activity.activity_type = $1
			AND activity.object_ap_id = $2
			AND delivery.target_inbox_url = $3
	`, activityType, objectAPID, inboxURL)
	require.NoError(t, err)
	require.Equal(t, expected, count)
}

func requireProjectDelivery(t *testing.T, deliveries []delivery.ProjectDelivery, activityType, objectAPID, inboxURL string) {
	t.Helper()

	if findProjectDelivery(deliveries, activityType, objectAPID, inboxURL) != nil {
		return
	}
	t.Fatalf("project delivery not found for %s %s to %s", activityType, objectAPID, inboxURL)
}

func findProjectDelivery(deliveries []delivery.ProjectDelivery, activityType, objectAPID, inboxURL string) *delivery.ProjectDelivery {
	for _, item := range deliveries {
		if item.ActivityType != activityType || item.TargetInboxURL != inboxURL || item.ObjectAPID == nil {
			continue
		}
		if *item.ObjectAPID == objectAPID {
			return &item
		}
	}
	return nil
}

func requireNoDeliveryForObject(t *testing.T, db *sqlx.DB, activityType, objectAPID, inboxURL string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM activity_deliveries delivery
		JOIN ap_activities activity ON activity.id = delivery.activity_id
		WHERE activity.activity_type = $1
			AND activity.object_ap_id = $2
			AND delivery.target_inbox_url = $3
	`, activityType, objectAPID, inboxURL)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireInboxItem(t *testing.T, db *sqlx.DB, actorID, activityType, objectAPID string) {
	t.Helper()
	requireBoxItem(t, db, "actor_inbox_items", actorID, activityType, objectAPID)
}

func requireNoInboxActivity(t *testing.T, db *sqlx.DB, activityAPID string) {
	t.Helper()

	requireInboxActivityCount(t, db, activityAPID, 0)
}

func requireInboxActivityCount(t *testing.T, db *sqlx.DB, activityAPID string, expected int) {
	t.Helper()

	var count int
	err := db.Get(&count, `SELECT count(*) FROM actor_inbox_items WHERE activity_ap_id = $1`, activityAPID)
	require.NoError(t, err)
	require.Equal(t, expected, count)
}

func requireInboxItemForTarget(t *testing.T, db *sqlx.DB, actorID, activityType, targetAPID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM actor_inbox_items inbox
		JOIN ap_activities activity ON activity.id = inbox.activity_id
		WHERE inbox.actor_id = $1
			AND activity.activity_type = $2
			AND activity.target_ap_id = $3
	`, actorID, activityType, targetAPID)
	require.NoError(t, err)
	require.Greater(t, count, 0)
}

func requireOutboxItem(t *testing.T, db *sqlx.DB, actorID, activityType, objectAPID string) {
	t.Helper()
	requireBoxItem(t, db, "actor_outbox_items", actorID, activityType, objectAPID)
}

func requireBoxItem(t *testing.T, db *sqlx.DB, tableName, actorID, activityType, objectAPID string) {
	t.Helper()

	if tableName != "actor_inbox_items" && tableName != "actor_outbox_items" {
		t.Fatalf("unexpected box table %q", tableName)
	}

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM `+tableName+` box
		JOIN ap_activities activity ON activity.id = box.activity_id
		WHERE box.actor_id = $1
			AND activity.activity_type = $2
			AND activity.object_ap_id = $3
	`, actorID, activityType, objectAPID)
	require.NoError(t, err)
	require.Greater(t, count, 0)
}

func requireProjectActorEndpointTombstone(t *testing.T, db *sqlx.DB, cfg activitypub.Config, projectID, formerType string) {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/projects/"+projectID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(projectID)

	require.NoError(t, activitypub.NewHandler(db, cfg).GetProjectActor(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc))
	require.Equal(t, "Tombstone", doc["type"])
	require.Equal(t, formerType, doc["formerType"])
	require.NotContains(t, doc, "name")
	require.NotContains(t, doc, "content")
}

func requireProjectActivityPubCollections(t *testing.T, db *sqlx.DB, cfg activitypub.Config, projectID, projectAPID, ticketAPID, remoteFollowerAPID string) {
	t.Helper()

	outboxID := activitypub.Outbox(projectAPID)
	outbox := requireActivityPubDocument(t, db, cfg, "/projects/"+projectID+"/outbox")
	require.Equal(t, "OrderedCollection", outbox["type"])
	require.Equal(t, outboxID, outbox["id"])
	require.NotContains(t, outbox, "orderedItems")
	require.Contains(t, outbox["first"], "page=true")
	outboxTotal := requireJSONInt(t, outbox, "totalItems")
	require.GreaterOrEqual(t, outboxTotal, 0)

	outboxPage := requireActivityPubDocument(t, db, cfg, "/projects/"+projectID+"/outbox?page=true&limit=2")
	require.Equal(t, "OrderedCollectionPage", outboxPage["type"])
	require.Equal(t, outboxID, outboxPage["partOf"])
	require.Equal(t, outbox["totalItems"], outboxPage["totalItems"])
	outboxItems := requireOrderedItems(t, outboxPage)
	require.LessOrEqual(t, len(outboxItems), 2)
	if outboxTotal > len(outboxItems) {
		require.Contains(t, outboxPage, "next")
	}

	inboxID := activitypub.Inbox(projectAPID)
	inbox := requireActivityPubDocument(t, db, cfg, "/projects/"+projectID+"/inbox")
	require.Equal(t, "OrderedCollection", inbox["type"])
	require.Equal(t, inboxID, inbox["id"])
	require.NotContains(t, inbox, "orderedItems")
	inboxTotal := requireJSONInt(t, inbox, "totalItems")
	require.GreaterOrEqual(t, inboxTotal, 1)

	inboxPage := requireActivityPubDocument(t, db, cfg, "/projects/"+projectID+"/inbox?page=true&limit=2")
	require.Equal(t, "OrderedCollectionPage", inboxPage["type"])
	require.Equal(t, inboxID, inboxPage["partOf"])
	require.Equal(t, inbox["totalItems"], inboxPage["totalItems"])
	inboxItems := requireOrderedItems(t, inboxPage)
	require.NotEmpty(t, inboxItems)
	require.LessOrEqual(t, len(inboxItems), 2)
	if inboxTotal > len(inboxItems) {
		require.Contains(t, inboxPage, "next")
	}

	followersID := activitypub.Followers(projectAPID)
	followers := requireActivityPubDocument(t, db, cfg, "/projects/"+projectID+"/followers")
	require.Equal(t, "OrderedCollection", followers["type"])
	require.Equal(t, followersID, followers["id"])
	require.NotContains(t, followers, "orderedItems")
	require.GreaterOrEqual(t, requireJSONInt(t, followers, "totalItems"), 3)

	followersPage := requireActivityPubDocument(t, db, cfg, "/projects/"+projectID+"/followers?page=true&limit=50")
	require.Equal(t, "OrderedCollectionPage", followersPage["type"])
	require.Equal(t, followersID, followersPage["partOf"])
	require.Contains(t, requireOrderedItems(t, followersPage), remoteFollowerAPID)

	ticketsID := activitypub.ProjectTickets(projectAPID)
	tickets := requireActivityPubDocument(t, db, cfg, "/projects/"+projectID+"/tickets")
	require.Equal(t, "OrderedCollection", tickets["type"])
	require.Equal(t, ticketsID, tickets["id"])
	require.NotContains(t, tickets, "orderedItems")
	ticketsTotal := requireJSONInt(t, tickets, "totalItems")
	require.GreaterOrEqual(t, ticketsTotal, 1)

	ticketsPage := requireActivityPubDocument(t, db, cfg, "/projects/"+projectID+"/tickets?page=true&limit=2")
	require.Equal(t, "OrderedCollectionPage", ticketsPage["type"])
	require.Equal(t, ticketsID, ticketsPage["partOf"])
	require.Equal(t, tickets["totalItems"], ticketsPage["totalItems"])
	ticketItems := requireOrderedItems(t, ticketsPage)
	require.Contains(t, ticketItems, ticketAPID)
	require.LessOrEqual(t, len(ticketItems), 2)
}

func requireC2SCreateTicket(t *testing.T, db *sqlx.DB, cfg activitypub.Config, ticketService *ticket.Service, commentService *comment.Service, actorID, username, actorAPID, projectAPID, remoteInbox string) string {
	t.Helper()

	body := map[string]any{
		"@context": activitypub.Context(),
		"type":     "Create",
		"actor":    actorAPID,
		"object": map[string]any{
			"type":             "forge:Ticket",
			"attributedTo":     actorAPID,
			"context":          projectAPID,
			"name":             "C2S protocol task",
			"content":          "Created through the local ActivityPub outbox.",
			"forge:priority":   "medium",
			"forge:ticketType": "task",
			"forge:isResolved": false,
		},
	}
	rec := postC2SOutbox(t, db, cfg, ticketService, commentService, username, actorID, body)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Contains(t, rec.Header().Get(echo.HeaderContentType), activitypub.ActivityJSONMediaType)

	var activity map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &activity))
	require.Equal(t, "Create", activity["type"])
	require.Equal(t, actorAPID, activity["actor"])
	require.Equal(t, rec.Header().Get(echo.HeaderLocation), activity["id"])

	ticketAPID, ok := activity["object"].(string)
	require.True(t, ok)
	require.NotEmpty(t, ticketAPID)

	requireObjectType(t, db, ticketAPID, "Ticket")
	requireTicketObjectName(t, db, ticketAPID, "C2S protocol task")
	requireActivityForObjectAndActor(t, db, "Create", ticketAPID, actorID)
	requireOutboxItem(t, db, actorID, "Create", ticketAPID)
	requireDeliveryForObject(t, db, "Create", ticketAPID, remoteInbox)
	return ticketAPID
}

func requireC2SCreateNote(t *testing.T, db *sqlx.DB, cfg activitypub.Config, ticketService *ticket.Service, commentService *comment.Service, actorID, username, actorAPID, ticketAPID, projectID, remoteInbox string) string {
	t.Helper()

	body := map[string]any{
		"@context": activitypub.Context(),
		"type":     "Create",
		"actor":    actorAPID,
		"object": map[string]any{
			"type":         "Note",
			"attributedTo": actorAPID,
			"inReplyTo":    ticketAPID,
			"content":      "C2S comment landed.",
		},
	}
	rec := postC2SOutbox(t, db, cfg, ticketService, commentService, username, actorID, body)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Contains(t, rec.Header().Get(echo.HeaderContentType), activitypub.ActivityJSONMediaType)

	var activity map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &activity))
	require.Equal(t, "Create", activity["type"])
	require.Equal(t, actorAPID, activity["actor"])
	require.Equal(t, rec.Header().Get(echo.HeaderLocation), activity["id"])

	object, ok := activity["object"].(map[string]any)
	require.True(t, ok)
	commentAPID, ok := object["id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, commentAPID)
	require.Equal(t, "Note", object["type"])
	require.Equal(t, ticketAPID, object["inReplyTo"])
	require.Equal(t, "C2S comment landed.", object["content"])

	requireObjectType(t, db, commentAPID, "Note")
	requireCommentContent(t, db, commentAPID, "C2S comment landed.")
	requireActivityForObjectAndActor(t, db, "Create", commentAPID, actorID)
	requireOutboxItem(t, db, actorID, "Create", commentAPID)
	requireInboxItem(t, db, projectID, "Create", commentAPID)
	requireDeliveryForObject(t, db, "Create", commentAPID, remoteInbox)
	return commentAPID
}

func requireC2SOutboxRejectsActorMismatch(t *testing.T, db *sqlx.DB, cfg activitypub.Config, ticketService *ticket.Service, commentService *comment.Service, actorID, username, wrongActorAPID, ticketAPID string) {
	t.Helper()

	before := activityCount(t, db)
	body := map[string]any{
		"type":  "Create",
		"actor": wrongActorAPID,
		"object": map[string]any{
			"type":      "Note",
			"inReplyTo": ticketAPID,
			"content":   "This should be rejected.",
		},
	}
	rec := postC2SOutbox(t, db, cfg, ticketService, commentService, username, actorID, body)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	requireActivityCount(t, db, before)
}

func postC2SOutbox(t *testing.T, db *sqlx.DB, cfg activitypub.Config, ticketService *ticket.Service, commentService *comment.Service, username, userID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()

	raw, err := json.Marshal(body)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/users/"+username+"/outbox", bytes.NewReader(raw))
	req.Header.Set(echo.HeaderContentType, activitypub.ActivityJSONMediaType)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("username")
	c.SetParamValues(username)
	c.Set("userID", userID)

	require.NoError(t, c2s.NewHandler(db, cfg, ticketService, commentService).PostUserOutbox(c))
	return rec
}

func requireActivityPubDocument(t *testing.T, db *sqlx.DB, cfg activitypub.Config, path string) map[string]any {
	t.Helper()

	e := echo.New()
	activitypub.NewHandler(db, cfg).RegisterRoutes(e)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Header().Get(echo.HeaderContentType), activitypub.ActivityJSONMediaType)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc))
	return doc
}

func requireProjectTicketsCollectionNotContains(t *testing.T, db *sqlx.DB, cfg activitypub.Config, projectID, projectAPID, ticketAPID string) {
	t.Helper()

	ticketsID := activitypub.ProjectTickets(projectAPID)
	ticketsPage := requireActivityPubDocument(t, db, cfg, "/projects/"+projectID+"/tickets?page=true&limit=50")
	require.Equal(t, "OrderedCollectionPage", ticketsPage["type"])
	require.Equal(t, ticketsID, ticketsPage["partOf"])
	require.NotContains(t, requireOrderedItems(t, ticketsPage), ticketAPID)
}

func requireProjectTicketsCollectionContains(t *testing.T, db *sqlx.DB, cfg activitypub.Config, projectID, projectAPID, ticketAPID string) {
	t.Helper()

	ticketsID := activitypub.ProjectTickets(projectAPID)
	ticketsPage := requireActivityPubDocument(t, db, cfg, "/projects/"+projectID+"/tickets?page=true&limit=50")
	require.Equal(t, "OrderedCollectionPage", ticketsPage["type"])
	require.Equal(t, ticketsID, ticketsPage["partOf"])
	require.Contains(t, requireOrderedItems(t, ticketsPage), ticketAPID)
}

func requireOrderedItems(t *testing.T, doc map[string]any) []any {
	t.Helper()

	items, ok := doc["orderedItems"].([]any)
	require.True(t, ok, "orderedItems should be an array")
	return items
}

func requireJSONInt(t *testing.T, doc map[string]any, key string) int {
	t.Helper()

	value, ok := doc[key].(float64)
	require.True(t, ok, "%s should be a JSON number", key)
	return int(value)
}

func requireTicketResolved(t *testing.T, db *sqlx.DB, ticketID string) {
	t.Helper()

	var resolved sql.NullBool
	err := db.Get(&resolved, `SELECT is_resolved FROM tickets WHERE id = $1`, ticketID)
	require.NoError(t, err)
	require.True(t, resolved.Valid)
	require.True(t, resolved.Bool)
}

func requireTicketResolvedBy(t *testing.T, db *sqlx.DB, ticketID, actorID string) {
	t.Helper()

	var resolvedBy string
	err := db.Get(&resolvedBy, `SELECT resolved_by_actor_id::text FROM tickets WHERE id = $1`, ticketID)
	require.NoError(t, err)
	require.Equal(t, actorID, resolvedBy)
}

func requireTicketAssignee(t *testing.T, db *sqlx.DB, ticketAPID, assigneeID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM ticket_assignees assignee
		JOIN tickets ticket ON ticket.id = assignee.ticket_id
		WHERE ticket.ap_id = $1 AND assignee.actor_id = $2
	`, ticketAPID, assigneeID)
	require.NoError(t, err)
	require.Greater(t, count, 0)
}

func requireNoTicketAssignee(t *testing.T, db *sqlx.DB, ticketAPID, assigneeID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM ticket_assignees assignee
		JOIN tickets ticket ON ticket.id = assignee.ticket_id
		WHERE ticket.ap_id = $1 AND assignee.actor_id = $2
	`, ticketAPID, assigneeID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireTicketObjectAssignedTo(t *testing.T, db *sqlx.DB, ticketAPID, assigneeAPID string) {
	t.Helper()

	var assigned bool
	err := db.Get(&assigned, `
		SELECT COALESCE(document->'forge:assignedTo' ? $2, false)
		FROM ap_objects
		WHERE ap_id = $1
	`, ticketAPID, assigneeAPID)
	require.NoError(t, err)
	require.True(t, assigned)
}

func requireTicketObjectNotAssignedTo(t *testing.T, db *sqlx.DB, ticketAPID, assigneeAPID string) {
	t.Helper()

	var assigned bool
	err := db.Get(&assigned, `
		SELECT COALESCE(document->'forge:assignedTo' ? $2, false)
		FROM ap_objects
		WHERE ap_id = $1
	`, ticketAPID, assigneeAPID)
	require.NoError(t, err)
	require.False(t, assigned)
}

func intPtr(value int) *int {
	return &value
}

func TestIntegrationDBSourceLooksSafe(t *testing.T) {
	source := os.Getenv("TEST_DB_SOURCE")
	if source == "" {
		t.Skip("set TEST_DB_SOURCE to check integration database safety")
	}
	require.True(t, strings.Contains(source, "localhost") || strings.Contains(source, "127.0.0.1") || strings.Contains(source, "db:5432"))
}
