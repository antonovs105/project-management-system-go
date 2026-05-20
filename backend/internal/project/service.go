package project

import (
	"context"
	"errors"
	"log"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
)

type Service struct {
	repo     Repository
	apConfig activitypub.Config
}

func NewService(repo Repository, apConfig activitypub.Config) *Service {
	return &Service{
		repo:     repo,
		apConfig: apConfig,
	}
}

func (s *Service) CreateProject(ctx context.Context, name, description string, userID string) (*Project, error) {
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

func (s *Service) GetProjectByID(ctx context.Context, projectID, userID string) (*Project, error) {
	project, err := s.repo.GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	role, err := s.GetProjectRole(ctx, projectID, userID)
	if err != nil {
		return nil, errors.New("project not found or access denied")
	}

	log.Printf("User %s has role '%s' in project %s", userID, role, projectID)
	return project, nil
}

func (s *Service) GetProjectRole(ctx context.Context, projectID, userID string) (string, error) {
	return s.repo.GetUserRole(ctx, userID, projectID)
}

func (s *Service) ListUserProjects(ctx context.Context, userID string) ([]Project, error) {
	return s.repo.ListByOwnerID(ctx, userID)
}

type UpdateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

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
		projectToUpdate.Name = *req.Name
	}
	if req.Description != nil {
		projectToUpdate.Description = *req.Description
	}

	return s.repo.Update(ctx, projectToUpdate)
}

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
	return s.repo.Delete(ctx, projectID)
}

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
		return s.repo.RemoveMember(ctx, projectID, actorID, targetUserID)
	}

	switch actorRole {
	case RoleOwner:
		return s.repo.RemoveMember(ctx, projectID, actorID, targetUserID)
	case RoleManager:
		if targetRole == RoleDeveloper || targetRole == RoleViewer {
			return s.repo.RemoveMember(ctx, projectID, actorID, targetUserID)
		}
		return errors.New("insufficient permissions: managers can only remove developers or viewers")
	default:
		return errors.New("insufficient permissions: only owners or managers can remove members")
	}
}

func (s *Service) AddMemberToProject(ctx context.Context, projectID, currentUserID, newUserID string, role string) (*ProjectInvite, error) {
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
		return nil, errors.New("invalid project role")
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
	if err := s.repo.CreateInvite(ctx, invite); err != nil {
		return nil, err
	}
	return invite, nil
}

func (s *Service) AcceptInvite(ctx context.Context, inviteID, userID string) error {
	return s.repo.AcceptInvite(ctx, inviteID, userID)
}

func (s *Service) RejectInvite(ctx context.Context, inviteID, userID string) error {
	return s.repo.RejectInvite(ctx, inviteID, userID)
}

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
	return s.repo.RevokeInvite(ctx, inviteID, userID)
}
