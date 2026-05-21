package project

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
)

// ErrInvalidProjectInput reports malformed project-management input.
var ErrInvalidProjectInput = errors.New("invalid project input")

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

	if _, err := s.GetProjectRole(ctx, projectID, userID); err != nil {
		return nil, errors.New("project not found or access denied")
	}

	return project, nil
}

// GetProjectRole returns the user's role in a project.
func (s *Service) GetProjectRole(ctx context.Context, projectID, userID string) (string, error) {
	return s.repo.GetUserRole(ctx, userID, projectID)
}

// ListUserProjects returns projects where the user is a member.
func (s *Service) ListUserProjects(ctx context.Context, userID string) ([]Project, error) {
	return s.repo.ListByOwnerID(ctx, userID)
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
	role, err := s.GetProjectRole(ctx, projectID, userID)
	if err != nil {
		return errors.New("project not found or access denied")
	}
	if !CanManageProject(role) {
		return errors.New("insufficient permissions: only owners or managers can update projects")
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
	role, err := s.GetProjectRole(ctx, projectID, userID)
	if err != nil {
		return errors.New("project not found or access denied")
	}
	if !CanDeleteProject(role) {
		return errors.New("insufficient permissions: only owners can delete projects")
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
	actorRole, err := s.repo.GetUserRole(ctx, actorID, projectID)
	if err != nil {
		return errors.New("access denied: you are not a member of this project")
	}
	targetRole, err := s.repo.GetUserRole(ctx, targetUserID, projectID)
	if err != nil {
		return errors.New("target user is not a project member")
	}

	if actorID == targetUserID {
		return s.removeMember(ctx, projectID, actorID, targetUserID)
	}

	switch actorRole {
	case RoleOwner:
		return s.removeMember(ctx, projectID, actorID, targetUserID)
	case RoleManager:
		if targetRole == RoleDeveloper || targetRole == RoleViewer {
			return s.removeMember(ctx, projectID, actorID, targetUserID)
		}
		return errors.New("insufficient permissions: managers can only remove developers or viewers")
	default:
		return errors.New("insufficient permissions: only owners or managers can remove members")
	}
}

// AddMemberToProject creates a pending project invite for a local user.
func (s *Service) AddMemberToProject(ctx context.Context, projectID, currentUserID, newUserID string, role string) (*ProjectInvite, error) {
	if strings.TrimSpace(newUserID) == "" {
		return nil, invalidProjectInput("user_id is required")
	}
	currentUserRole, err := s.repo.GetUserRole(ctx, currentUserID, projectID)
	if err != nil {
		return nil, errors.New("access denied: you are not a member of this project")
	}

	if !CanManageMembers(currentUserRole) {
		return nil, errors.New("insufficient permissions: only owners or managers can invite new members")
	}

	if role == "" {
		role = RoleViewer
	}
	if !IsValidRole(role) {
		return nil, invalidProjectInput("invalid project role")
	}
	if role == RoleOwner && currentUserRole != RoleOwner {
		return nil, errors.New("insufficient permissions: only owners can invite owners")
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
		Role:           role,
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

// invalidProjectInput wraps a validation message with the project input sentinel.
func invalidProjectInput(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidProjectInput, message)
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
	role, err := s.repo.GetUserRole(ctx, userID, invite.ProjectID)
	if err != nil {
		return errors.New("access denied: you are not a member of this project")
	}
	if !CanManageMembers(role) {
		return errors.New("insufficient permissions: only owners or managers can revoke invites")
	}
	if invite.Role == RoleOwner && role != RoleOwner {
		return errors.New("insufficient permissions: only owners can revoke owner invites")
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
