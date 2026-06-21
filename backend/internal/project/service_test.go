package project

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type testDeliveryEnqueuer struct {
	deliveries []apdelivery.QueueCandidate
}

func (d *testDeliveryEnqueuer) EnqueuePersisted(ctx context.Context, deliveries []apdelivery.QueueCandidate) error {
	d.deliveries = append(d.deliveries, deliveries...)
	return nil
}

func TestService_CreateProject(t *testing.T) {
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

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

	t.Run("InvalidName", func(t *testing.T) {
		p, err := service.CreateProject(ctx, " ", desc, userID)

		assert.ErrorIs(t, err, ErrInvalidProjectInput)
		assert.Nil(t, p)
	})

	t.Run("RejectsOversizedName", func(t *testing.T) {
		p, err := service.CreateProject(ctx, strings.Repeat("x", maxProjectNameLength+1), desc, userID)

		assert.ErrorIs(t, err, ErrInvalidProjectInput)
		assert.Nil(t, p)
		assert.Contains(t, err.Error(), "name must be at most")
	})

	t.Run("RejectsOversizedDescription", func(t *testing.T) {
		p, err := service.CreateProject(ctx, name, strings.Repeat("x", maxProjectDescriptionLength+1), userID)

		assert.ErrorIs(t, err, ErrInvalidProjectInput)
		assert.Nil(t, p)
		assert.Contains(t, err.Error(), "description must be at most")
	})
}

func TestService_UpdateProjectValidatesMetadataLength(t *testing.T) {
	ctx := context.Background()
	projectID := "project-1"
	userID := "user-1"

	t.Run("RejectsOversizedName", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
		longName := strings.Repeat("x", maxProjectNameLength+1)

		mockRepo.On("GetByID", ctx, projectID).Return(&Project{ID: projectID, Name: "Test Project", OwnerID: userID}, nil).Once()
		mockRepo.On("HasPermission", ctx, projectID, userID, PermissionProjectRead).Return(true, nil).Once()
		mockRepo.On("HasPermission", ctx, projectID, userID, PermissionProjectUpdate).Return(true, nil).Once()

		err := service.UpdateProject(ctx, projectID, userID, UpdateProjectRequest{Name: &longName})

		assert.ErrorIs(t, err, ErrInvalidProjectInput)
		assert.Contains(t, err.Error(), "name must be at most")
		mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything)
		mockRepo.AssertExpectations(t)
	})

	t.Run("RejectsOversizedDescription", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
		longDescription := strings.Repeat("x", maxProjectDescriptionLength+1)

		mockRepo.On("GetByID", ctx, projectID).Return(&Project{ID: projectID, Name: "Test Project", OwnerID: userID}, nil).Once()
		mockRepo.On("HasPermission", ctx, projectID, userID, PermissionProjectRead).Return(true, nil).Once()
		mockRepo.On("HasPermission", ctx, projectID, userID, PermissionProjectUpdate).Return(true, nil).Once()

		err := service.UpdateProject(ctx, projectID, userID, UpdateProjectRequest{Description: &longDescription})

		assert.ErrorIs(t, err, ErrInvalidProjectInput)
		assert.Contains(t, err.Error(), "description must be at most")
		mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything)
		mockRepo.AssertExpectations(t)
	})
}

func TestService_GetProjectByID(t *testing.T) {
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

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
		mockRepo.On("HasPermission", ctx, projectID, userID, PermissionProjectRead).Return(true, nil).Once()

		p, err := service.GetProjectByID(ctx, projectID, userID)

		assert.NoError(t, err)
		assert.Equal(t, expectedProject, p)
		mockRepo.AssertExpectations(t)
	})

	t.Run("AccessDenied", func(t *testing.T) {
		mockRepo.On("GetByID", ctx, projectID).Return(expectedProject, nil).Once()
		mockRepo.On("HasPermission", ctx, projectID, userID, PermissionProjectRead).Return(false, nil).Once()

		p, err := service.GetProjectByID(ctx, projectID, userID)

		assert.Error(t, err)
		assert.Nil(t, p)
		assert.Contains(t, err.Error(), "project not found or access denied")
		mockRepo.AssertExpectations(t)
	})
}

func TestService_ListProjectMembersRequiresProjectRead(t *testing.T) {
	ctx := context.Background()
	projectID := "project-1"
	userID := "manager-1"

	t.Run("allowed by project read permission", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
		expected := []ProjectMember{{UserID: "member-1", ProjectID: projectID, Role: RoleDeveloper}}

		mockRepo.On("HasPermission", ctx, projectID, userID, PermissionProjectRead).Return(true, nil).Once()
		mockRepo.On("ListMembers", ctx, projectID, ProjectListOptions{Limit: defaultProjectListLimit, Offset: 0}).Return(expected, nil).Once()

		members, err := service.ListProjectMembers(ctx, projectID, userID, ProjectListOptions{})

		assert.NoError(t, err)
		assert.Equal(t, expected, members)
		mockRepo.AssertExpectations(t)
	})

	t.Run("denied without project read permission", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

		mockRepo.On("HasPermission", ctx, projectID, userID, PermissionProjectRead).Return(false, nil).Once()

		members, err := service.ListProjectMembers(ctx, projectID, userID, ProjectListOptions{})

		assert.Error(t, err)
		assert.Nil(t, members)
		assert.Contains(t, err.Error(), "project not found or access denied")
		mockRepo.AssertNotCalled(t, "ListMembers", mock.Anything, mock.Anything, mock.Anything)
		mockRepo.AssertExpectations(t)
	})
}

func TestService_ListProjectInvitesValidatesStatusAndNormalizesPagination(t *testing.T) {
	ctx := context.Background()
	projectID := "project-1"
	userID := "manager-1"
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	expected := []ProjectInviteInspection{{ID: "invite-1", ProjectID: projectID, Status: "pending"}}

	mockRepo.On("HasPermission", ctx, projectID, userID, PermissionMembersInvite).Return(true, nil).Once()
	mockRepo.On("ListInvites", ctx, projectID, ProjectInviteListOptions{Status: "pending", Limit: 25, Offset: 5}).Return(expected, nil).Once()

	invites, err := service.ListProjectInvites(ctx, projectID, userID, ProjectInviteListOptions{Status: " PENDING ", Limit: 25, Offset: 5})

	assert.NoError(t, err)
	assert.Equal(t, expected, invites)
	mockRepo.AssertExpectations(t)
}

func TestService_ListProjectInvitesRejectsInvalidStatus(t *testing.T) {
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

	invites, err := service.ListProjectInvites(context.Background(), "project-1", "manager-1", ProjectInviteListOptions{Status: "waiting"})

	assert.ErrorIs(t, err, ErrInvalidProjectInput)
	assert.Nil(t, invites)
	mockRepo.AssertNotCalled(t, "HasPermission", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockRepo.AssertNotCalled(t, "ListInvites", mock.Anything, mock.Anything, mock.Anything)
}

func TestService_ListUserInvitesNormalizesFiltersWithoutProjectPermission(t *testing.T) {
	ctx := context.Background()
	userID := "invitee-1"
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	expected := []ProjectInviteInspection{{ID: "invite-1", InviteeActorID: userID, Status: "pending"}}

	mockRepo.On("ListInvitesForActor", ctx, userID, ProjectInviteListOptions{Status: "pending", Limit: defaultProjectListLimit, Offset: 0}).Return(expected, nil).Once()

	invites, err := service.ListUserInvites(ctx, userID, ProjectInviteListOptions{Status: " PENDING "})

	assert.NoError(t, err)
	assert.Equal(t, expected, invites)
	mockRepo.AssertNotCalled(t, "HasPermission", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockRepo.AssertExpectations(t)
}

func TestService_UpdateProjectMemberRoleRequiresRoleManagement(t *testing.T) {
	ctx := context.Background()
	projectID := "project-1"
	actorID := "manager-1"
	targetUserID := "member-1"
	roleID := "role-1"
	role := &ProjectRole{ID: roleID, ProjectID: projectID, Key: RoleDeveloper, Permissions: []string{PermissionProjectRead}}
	expected := &ProjectMember{UserID: targetUserID, ProjectID: projectID, RoleID: roleID, Role: RoleDeveloper}

	t.Run("success", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

		mockRepo.On("HasPermission", ctx, projectID, actorID, PermissionRolesManage).Return(true, nil).Once()
		mockRepo.On("ResolveRole", ctx, projectID, RoleDeveloper).Return(role, nil).Once()
		mockRepo.On("UpdateMemberRole", ctx, projectID, targetUserID, roleID).Return(expected, nil).Once()

		member, err := service.UpdateProjectMemberRole(ctx, projectID, actorID, targetUserID, RoleDeveloper)

		assert.NoError(t, err)
		assert.Equal(t, expected, member)
		mockRepo.AssertExpectations(t)
	})

	t.Run("denied", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

		mockRepo.On("HasPermission", ctx, projectID, actorID, PermissionRolesManage).Return(false, nil).Once()

		member, err := service.UpdateProjectMemberRole(ctx, projectID, actorID, targetUserID, RoleDeveloper)

		assert.Error(t, err)
		assert.Nil(t, member)
		assert.Contains(t, err.Error(), "insufficient permissions")
		mockRepo.AssertNotCalled(t, "ResolveRole", mock.Anything, mock.Anything, mock.Anything)
		mockRepo.AssertNotCalled(t, "UpdateMemberRole", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockRepo.AssertExpectations(t)
	})
}

func TestService_UpdateProjectMemberRoleRejectsBlankRole(t *testing.T) {
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

	member, err := service.UpdateProjectMemberRole(context.Background(), "project-1", "manager-1", "member-1", " ")

	assert.ErrorIs(t, err, ErrInvalidProjectInput)
	assert.Nil(t, member)
	mockRepo.AssertNotCalled(t, "HasPermission", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestService_AddMemberToProjectResolvesHumanInviteeReference(t *testing.T) {
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	delivery := &testDeliveryEnqueuer{}
	service.SetDelivery(delivery)
	ctx := context.Background()
	projectID := "project-1"
	actorID := "owner-1"
	inviteeID := "invitee-1"
	role := &ProjectRole{ID: "role-1", ProjectID: projectID, Key: RoleViewer, Name: "Viewer", Permissions: []string{PermissionProjectRead}}

	mockRepo.On("HasPermission", ctx, projectID, actorID, PermissionMembersInvite).Return(true, nil).Once()
	mockRepo.On("ResolveInviteeActorID", ctx, "alice@example.test").Return(inviteeID, nil).Once()
	mockRepo.On("ResolveRole", ctx, projectID, RoleViewer).Return(role, nil).Once()
	mockRepo.On("IsProjectMember", ctx, projectID, inviteeID).Return(false, nil).Once()
	mockRepo.On("HasPendingInvite", ctx, projectID, inviteeID).Return(false, nil).Once()
	mockRepo.On("CreateInvite", ctx, mock.MatchedBy(func(invite *ProjectInvite) bool {
		return invite.ProjectID == projectID &&
			invite.InviterActorID == actorID &&
			invite.InviteeActorID == inviteeID &&
			invite.RoleID == role.ID &&
			invite.Role == role.Key &&
			invite.Status == "pending"
	})).Return(&MembershipResult{
		ProjectID:  projectID,
		Deliveries: []apdelivery.QueueCandidate{{ID: "delivery-1", MaxAttempts: 10}},
	}, nil).Once()

	invite, err := service.AddMemberToProject(ctx, projectID, actorID, " alice@example.test ", "")

	assert.NoError(t, err)
	assert.NotNil(t, invite)
	assert.Equal(t, inviteeID, invite.InviteeActorID)
	assert.Equal(t, []apdelivery.QueueCandidate{{ID: "delivery-1", MaxAttempts: 10}}, delivery.deliveries)
	mockRepo.AssertExpectations(t)
}

func TestService_UpdateProjectRoleRejectsRemovingLastRoleManager(t *testing.T) {
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	ctx := context.Background()
	projectID := "project-1"
	userID := "manager-1"
	roleID := "role-1"
	permissions := []string{PermissionProjectRead}
	existingRole := &ProjectRole{
		ID:          roleID,
		ProjectID:   projectID,
		Name:        "Coordinator",
		Permissions: []string{PermissionProjectRead, PermissionRolesManage},
	}

	mockRepo.On("HasPermission", ctx, projectID, userID, PermissionRolesManage).Return(true, nil).Once()
	mockRepo.On("GetRoleByID", ctx, projectID, roleID).Return(existingRole, nil).Once()
	mockRepo.On("CountMembersWithPermissionExcludingRole", ctx, projectID, PermissionRolesManage, roleID).Return(0, nil).Once()

	role, err := service.UpdateProjectRole(ctx, projectID, userID, roleID, UpdateProjectRoleRequest{Permissions: &permissions})

	assert.Error(t, err)
	assert.Equal(t, "cannot remove the last project role manager", err.Error())
	assert.Nil(t, role)
	mockRepo.AssertNotCalled(t, "UpdateRole", mock.Anything, mock.Anything)
	mockRepo.AssertExpectations(t)
}

func TestService_UpdateProjectRoleAllowsRemovingRoleManagementWhenAnotherManagerRemains(t *testing.T) {
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	ctx := context.Background()
	projectID := "project-1"
	userID := "manager-1"
	roleID := "role-1"
	permissions := []string{PermissionProjectRead}
	existingRole := &ProjectRole{
		ID:          roleID,
		ProjectID:   projectID,
		Name:        "Coordinator",
		Permissions: []string{PermissionProjectRead, PermissionRolesManage},
	}

	mockRepo.On("HasPermission", ctx, projectID, userID, PermissionRolesManage).Return(true, nil).Once()
	mockRepo.On("GetRoleByID", ctx, projectID, roleID).Return(existingRole, nil).Once()
	mockRepo.On("CountMembersWithPermissionExcludingRole", ctx, projectID, PermissionRolesManage, roleID).Return(1, nil).Once()
	mockRepo.On("UpdateRole", ctx, mock.MatchedBy(func(role *ProjectRole) bool {
		return role.ID == roleID && !hasPermission(role.Permissions, PermissionRolesManage)
	})).Return(nil).Once()

	role, err := service.UpdateProjectRole(ctx, projectID, userID, roleID, UpdateProjectRoleRequest{Permissions: &permissions})

	assert.NoError(t, err)
	assert.NotNil(t, role)
	assert.False(t, hasPermission(role.Permissions, PermissionRolesManage))
	mockRepo.AssertExpectations(t)
}
