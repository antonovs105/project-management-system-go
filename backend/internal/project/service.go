package project

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
)

// ErrInvalidProjectInput reports malformed project-management input.
var ErrInvalidProjectInput = errors.New("invalid project input")

const (
	// defaultProjectListLimit is the fallback project list size.
	defaultProjectListLimit = 100
	// maxProjectListLimit is the largest accepted project list size.
	maxProjectListLimit = 500
)

// Service contains project board, membership, and invite workflows.
type Service struct {
	repo     Repository
	apConfig activitypub.Config
	delivery DeliveryEnqueuer
}

// DeliveryEnqueuer queues ActivityPub deliveries created by project actions.
type DeliveryEnqueuer interface {
	Enqueue(ctx context.Context, activityID string, targetInboxURL string) (*apdelivery.Delivery, error)
}

// NewService creates a project service.
func NewService(repo Repository, apConfig activitypub.Config) *Service {
	return &Service{
		repo:     repo,
		apConfig: apConfig,
	}
}

// SetDelivery attaches the delivery queue used for project federation.
func (s *Service) SetDelivery(delivery DeliveryEnqueuer) {
	s.delivery = delivery
}

// CreateProject creates a project actor, owner membership, and Create activity.
func (s *Service) CreateProject(ctx context.Context, name, description string, userID string) (*Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, invalidProjectInput("name is required")
	}

	projectID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	publicKey, privateKey, err := activitypub.GenerateRSAKeyPair()
	if err != nil {
		return nil, err
	}

	p := &Project{
		ID:            projectID,
		APID:          activitypub.ProjectAPID(s.apConfig, projectID),
		Name:          name,
		Description:   description,
		OwnerID:       userID,
		Handle:        activitypub.Handle("project-"+projectID, s.apConfig),
		PublicKeyPEM:  publicKey,
		PrivateKeyPEM: privateKey,
	}

	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// GetProjectByID returns a project visible to the given user.
func (s *Service) GetProjectByID(ctx context.Context, projectID, userID string) (*Project, error) {
	project, err := s.repo.GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if err := s.RequireProjectPermission(ctx, projectID, userID, PermissionProjectRead, "project not found or access denied"); err != nil {
		return nil, errors.New("project not found or access denied")
	}

	return project, nil
}

// GetProjectRole returns the user's role in a project.
func (s *Service) GetProjectRole(ctx context.Context, projectID, userID string) (string, error) {
	return s.repo.GetMemberRole(ctx, userID, projectID)
}

// HasProjectPermission reports whether userID has permission in projectID.
func (s *Service) HasProjectPermission(ctx context.Context, projectID, userID, permission string) (bool, error) {
	if !IsSupportedPermission(permission) {
		return false, invalidProjectInput("invalid project permission")
	}
	return s.repo.HasPermission(ctx, projectID, userID, permission)
}

// RequireProjectPermission returns a consistent access error when a project permission is missing.
func (s *Service) RequireProjectPermission(ctx context.Context, projectID, userID, permission, deniedMessage string) error {
	allowed, err := s.HasProjectPermission(ctx, projectID, userID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return errors.New(deniedMessage)
	}
	return nil
}

// ListUserProjects returns projects where the user is a member.
func (s *Service) ListUserProjects(ctx context.Context, userID string, options ProjectListOptions) ([]Project, error) {
	options.Limit = normalizeProjectListLimit(options.Limit)
	options.Offset = normalizeProjectListOffset(options.Offset)
	return s.repo.ListByOwnerID(ctx, userID, options)
}

// UpdateProjectRequest contains partial project metadata updates.
type UpdateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// UpdateProject changes project metadata and emits an Update activity.
func (s *Service) UpdateProject(ctx context.Context, projectID, userID string, req UpdateProjectRequest) error {
	projectToUpdate, err := s.GetProjectByID(ctx, projectID, userID)
	if err != nil {
		return err
	}
	if err := s.RequireProjectPermission(ctx, projectID, userID, PermissionProjectUpdate, "insufficient permissions: missing project.update"); err != nil {
		return err
	}

	if req.Name != nil {
		trimmedName := strings.TrimSpace(*req.Name)
		if trimmedName == "" {
			return invalidProjectInput("name is required")
		}
		req.Name = &trimmedName
		projectToUpdate.Name = *req.Name
	}
	if req.Description != nil {
		projectToUpdate.Description = *req.Description
	}

	updateResult, err := s.repo.Update(ctx, projectToUpdate, userID)
	if err != nil {
		return err
	}
	s.enqueueActivityRecipientInboxes(ctx, updateResult.ProjectID, updateResult.ActivityID, updateResult.RecipientInboxes)
	return nil
}

// DeleteProject deletes a project and emits ActivityPub tombstone side effects.
func (s *Service) DeleteProject(ctx context.Context, projectID, userID string) error {
	if _, err := s.GetProjectByID(ctx, projectID, userID); err != nil {
		return err
	}
	if err := s.RequireProjectPermission(ctx, projectID, userID, PermissionProjectDelete, "insufficient permissions: missing project.delete"); err != nil {
		return err
	}
	deleteResult, err := s.repo.Delete(ctx, projectID, userID)
	if err != nil {
		return err
	}
	s.enqueueActivityRecipientInboxes(ctx, deleteResult.ProjectID, deleteResult.ActivityID, deleteResult.RecipientInboxes)
	return nil
}

// enqueueActivityRecipientInboxes queues a project activity to explicit inbox URLs.
func (s *Service) enqueueActivityRecipientInboxes(ctx context.Context, projectID string, activityID string, inboxes []string) {
	if s.delivery == nil || activityID == "" {
		return
	}
	for _, inbox := range inboxes {
		if inbox == "" {
			continue
		}
		if _, err := s.delivery.Enqueue(ctx, activityID, inbox); err != nil {
			log.Printf("failed to enqueue ActivityPub delivery for project %s inbox %s: %v", projectID, inbox, err)
		}
	}
}

// removeMember removes a member and queues any resulting federation delivery.
func (s *Service) removeMember(ctx context.Context, projectID, actorID, targetUserID string) error {
	result, err := s.repo.RemoveMember(ctx, projectID, actorID, targetUserID)
	if err != nil {
		return err
	}
	if result != nil {
		s.enqueueActivityRecipientInboxes(ctx, result.ProjectID, result.ActivityID, result.RecipientInboxes)
	}
	return nil
}

// RemoveMemberFromProject removes a member when the acting user is allowed to do so.
func (s *Service) RemoveMemberFromProject(ctx context.Context, projectID, actorID, targetUserID string) error {
	if actorID == targetUserID {
		canManage, err := s.repo.HasPermission(ctx, projectID, targetUserID, PermissionRolesManage)
		if err != nil {
			return err
		}
		if canManage {
			managers, err := s.repo.CountMembersWithPermission(ctx, projectID, PermissionRolesManage)
			if err != nil {
				return err
			}
			if managers <= 1 {
				return errors.New("cannot remove the last project role manager")
			}
		}
		return s.removeMember(ctx, projectID, actorID, targetUserID)
	}

	if err := s.RequireProjectPermission(ctx, projectID, actorID, PermissionMembersRemove, "insufficient permissions: missing members.remove"); err != nil {
		return err
	}

	targetCanManage, err := s.repo.HasPermission(ctx, projectID, targetUserID, PermissionRolesManage)
	if err != nil {
		return errors.New("target user is not a project member")
	}
	if targetCanManage {
		if err := s.RequireProjectPermission(ctx, projectID, actorID, PermissionRolesManage, "insufficient permissions: missing roles.manage"); err != nil {
			return err
		}
		managers, err := s.repo.CountMembersWithPermission(ctx, projectID, PermissionRolesManage)
		if err != nil {
			return err
		}
		if managers <= 1 {
			return errors.New("cannot remove the last project role manager")
		}
	}

	return s.removeMember(ctx, projectID, actorID, targetUserID)
}

// AddMemberToProject creates a pending project invite for a local user.
func (s *Service) AddMemberToProject(ctx context.Context, projectID, currentUserID, newUserID string, roleRef string) (*ProjectInvite, error) {
	if strings.TrimSpace(newUserID) == "" {
		return nil, invalidProjectInput("user_id is required")
	}
	if err := s.RequireProjectPermission(ctx, projectID, currentUserID, PermissionMembersInvite, "insufficient permissions: missing members.invite"); err != nil {
		return nil, err
	}

	roleRef = strings.TrimSpace(roleRef)
	if roleRef == "" {
		roleRef = RoleViewer
	}
	role, err := s.repo.ResolveRole(ctx, projectID, roleRef)
	if err != nil {
		return nil, invalidProjectInput("invalid project role")
	}
	if grantsSensitiveProjectAccess(role) {
		if err := s.RequireProjectPermission(ctx, projectID, currentUserID, PermissionRolesManage, "insufficient permissions: missing roles.manage"); err != nil {
			return nil, err
		}
	}
	member, err := s.repo.IsProjectMember(ctx, projectID, newUserID)
	if err != nil {
		return nil, err
	}
	if member {
		return nil, errors.New("user is already a project member")
	}
	pending, err := s.repo.HasPendingInvite(ctx, projectID, newUserID)
	if err != nil {
		return nil, err
	}
	if pending {
		return nil, errors.New("pending invite already exists")
	}

	inviteID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	invite := &ProjectInvite{
		ID:             inviteID,
		ProjectID:      projectID,
		InviterActorID: currentUserID,
		InviteeActorID: newUserID,
		RoleID:         role.ID,
		Role:           role.Key,
		Status:         "pending",
	}
	result, err := s.repo.CreateInvite(ctx, invite)
	if err != nil {
		return nil, err
	}
	if result != nil {
		s.enqueueActivityRecipientInboxes(ctx, result.ProjectID, result.ActivityID, result.RecipientInboxes)
	}
	return invite, nil
}

// CreateProjectRoleRequest contains a project-local role definition.
type CreateProjectRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// UpdateProjectRoleRequest contains nullable project role updates.
type UpdateProjectRoleRequest struct {
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	Permissions *[]string `json:"permissions"`
}

// ListProjectRoles returns the project's configurable roles.
func (s *Service) ListProjectRoles(ctx context.Context, projectID, userID string) ([]ProjectRole, error) {
	if err := s.RequireProjectPermission(ctx, projectID, userID, PermissionProjectRead, "project not found or access denied"); err != nil {
		return nil, err
	}
	return s.repo.ListRoles(ctx, projectID)
}

// CreateProjectRole creates a project-local role controlled by a project role manager.
func (s *Service) CreateProjectRole(ctx context.Context, projectID, userID string, req CreateProjectRoleRequest) (*ProjectRole, error) {
	if err := s.RequireProjectPermission(ctx, projectID, userID, PermissionRolesManage, "insufficient permissions: missing roles.manage"); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, invalidProjectInput("role name is required")
	}
	permissions, err := normalizePermissions(req.Permissions)
	if err != nil {
		return nil, err
	}
	role := &ProjectRole{
		ProjectID:   projectID,
		Key:         normalizeRoleKey(name),
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Permissions: permissions,
	}
	if err := s.repo.CreateRole(ctx, role); err != nil {
		return nil, err
	}
	return role, nil
}

// UpdateProjectRole updates a project-local role controlled by a project role manager.
func (s *Service) UpdateProjectRole(ctx context.Context, projectID, userID, roleID string, req UpdateProjectRoleRequest) (*ProjectRole, error) {
	if err := s.RequireProjectPermission(ctx, projectID, userID, PermissionRolesManage, "insufficient permissions: missing roles.manage"); err != nil {
		return nil, err
	}
	role, err := s.repo.GetRoleByID(ctx, projectID, roleID)
	if err != nil {
		return nil, errors.New("project role not found")
	}
	if role.IsSystem {
		if req.Name != nil || req.Permissions != nil {
			return nil, errors.New("protected project role cannot be renamed or have permissions changed")
		}
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, invalidProjectInput("role name is required")
		}
		role.Name = name
	}
	if req.Description != nil {
		role.Description = strings.TrimSpace(*req.Description)
	}
	if req.Permissions != nil {
		permissions, err := normalizePermissions(*req.Permissions)
		if err != nil {
			return nil, err
		}
		role.Permissions = permissions
	}
	if err := s.repo.UpdateRole(ctx, role); err != nil {
		return nil, err
	}
	return role, nil
}

// DeleteProjectRole removes an unused custom project role.
func (s *Service) DeleteProjectRole(ctx context.Context, projectID, userID, roleID string) error {
	if err := s.RequireProjectPermission(ctx, projectID, userID, PermissionRolesManage, "insufficient permissions: missing roles.manage"); err != nil {
		return err
	}
	role, err := s.repo.GetRoleByID(ctx, projectID, roleID)
	if err != nil {
		return errors.New("project role not found")
	}
	if role.IsSystem || grantsSensitiveProjectAccess(role) {
		return errors.New("protected project role cannot be deleted")
	}
	assignments, err := s.repo.RoleAssignmentCount(ctx, projectID, roleID)
	if err != nil {
		return err
	}
	if assignments > 0 {
		return errors.New("project role is still assigned")
	}
	return s.repo.DeleteRole(ctx, projectID, roleID)
}

// grantsSensitiveProjectAccess reports whether a role can control project power boundaries.
func grantsSensitiveProjectAccess(role *ProjectRole) bool {
	for _, permission := range role.Permissions {
		if permission == PermissionRolesManage || permission == PermissionProjectDelete {
			return true
		}
	}
	return false
}

// normalizePermissions validates and de-duplicates project role permissions.
func normalizePermissions(raw []string) ([]string, error) {
	seen := make(map[string]struct{}, len(raw))
	for _, permission := range raw {
		permission = strings.TrimSpace(permission)
		if permission == "" {
			continue
		}
		if !IsSupportedPermission(permission) {
			return nil, invalidProjectInput("invalid project permission")
		}
		seen[permission] = struct{}{}
	}
	permissions := make([]string, 0, len(seen))
	for _, permission := range SupportedProjectPermissions {
		if _, ok := seen[permission]; ok {
			permissions = append(permissions, permission)
		}
	}
	return permissions, nil
}

// normalizeRoleKey derives a stable project role key from a display name.
func normalizeRoleKey(name string) string {
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastUnderscore = false
		case unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r):
			if builder.Len() > 0 && !lastUnderscore {
				builder.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	key := strings.Trim(builder.String(), "_")
	if key == "" {
		return "custom_role"
	}
	return key
}

// invalidProjectInput wraps a validation message with the project input sentinel.
func invalidProjectInput(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidProjectInput, message)
}

// normalizeProjectListLimit bounds project list sizes.
func normalizeProjectListLimit(limit int) int {
	if limit <= 0 {
		return defaultProjectListLimit
	}
	if limit > maxProjectListLimit {
		return maxProjectListLimit
	}
	return limit
}

// normalizeProjectListOffset clamps negative project list offsets.
func normalizeProjectListOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

// AcceptInvite accepts a pending project invite for the current user.
func (s *Service) AcceptInvite(ctx context.Context, inviteID, userID string) error {
	result, err := s.repo.AcceptInvite(ctx, inviteID, userID)
	if err != nil {
		return err
	}
	if result != nil {
		s.enqueueActivityRecipientInboxes(ctx, result.ProjectID, result.ActivityID, result.RecipientInboxes)
	}
	return nil
}

// RejectInvite rejects a pending project invite for the current user.
func (s *Service) RejectInvite(ctx context.Context, inviteID, userID string) error {
	result, err := s.repo.RejectInvite(ctx, inviteID, userID)
	if err != nil {
		return err
	}
	if result != nil {
		s.enqueueActivityRecipientInboxes(ctx, result.ProjectID, result.ActivityID, result.RecipientInboxes)
	}
	return nil
}

// RevokeInvite revokes a pending project invite when the actor can manage members.
func (s *Service) RevokeInvite(ctx context.Context, inviteID, userID string) error {
	invite, err := s.repo.GetInviteByID(ctx, inviteID)
	if err != nil {
		return err
	}
	if err := s.RequireProjectPermission(ctx, invite.ProjectID, userID, PermissionMembersInvite, "insufficient permissions: missing members.invite"); err != nil {
		return err
	}
	targetRole, err := s.repo.GetRoleByID(ctx, invite.ProjectID, invite.RoleID)
	if err != nil {
		return errors.New("project role not found")
	}
	if grantsSensitiveProjectAccess(targetRole) {
		if err := s.RequireProjectPermission(ctx, invite.ProjectID, userID, PermissionRolesManage, "insufficient permissions: missing roles.manage"); err != nil {
			return err
		}
	}
	result, err := s.repo.RevokeInvite(ctx, inviteID, userID)
	if err != nil {
		return err
	}
	if result != nil {
		s.enqueueActivityRecipientInboxes(ctx, result.ProjectID, result.ActivityID, result.RecipientInboxes)
	}
	return nil
}
