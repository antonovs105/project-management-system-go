package project

import "time"

const (
	// RoleOwner allows full control over a project and its membership.
	RoleOwner = "owner"
	// RoleManager allows project updates, member management, and ticket moderation.
	RoleManager = "manager"
	// RoleDeveloper allows ticket and comment work without member management.
	RoleDeveloper = "developer"
	// RoleViewer allows read-only access to project resources.
	RoleViewer = "viewer"
)

// Project is a local ActivityPub Group actor used as a project board.
type Project struct {
	ID            string    `db:"id" json:"id"`
	APID          string    `db:"ap_id" json:"ap_id"`
	Name          string    `db:"name" json:"name"`
	Description   string    `db:"description" json:"description"`
	OwnerID       string    `db:"owner_id" json:"owner_id"`
	Handle        string    `db:"handle" json:"handle"`
	PublicKeyPEM  string    `db:"public_key_pem" json:"-"`
	PrivateKeyPEM string    `db:"private_key_pem" json:"-"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

// ProjectInvite represents an Invite activity awaiting an accept, reject, or revoke response.
type ProjectInvite struct {
	ID             string    `db:"id" json:"id"`
	APID           string    `db:"ap_id" json:"ap_id"`
	ProjectID      string    `db:"project_id" json:"project_id"`
	InviterActorID string    `db:"inviter_actor_id" json:"inviter_actor_id"`
	InviteeActorID string    `db:"invitee_actor_id" json:"invitee_actor_id"`
	Role           string    `db:"role" json:"role"`
	Status         string    `db:"status" json:"status"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}

// ProjectMember is the relational projection of a user's role in a project.
type ProjectMember struct {
	UserID    string    `db:"user_id" json:"user_id"`
	ProjectID string    `db:"project_id" json:"project_id"`
	Role      string    `db:"role" json:"role"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// ProjectListOptions contains pagination for project list responses.
type ProjectListOptions struct {
	Limit  int
	Offset int
}

// UpdateResult carries the ActivityPub side effects produced by a project update.
type UpdateResult struct {
	ActivityID       string
	ProjectID        string
	RecipientInboxes []string
}

// DeleteResult carries the ActivityPub side effects produced by project deletion.
type DeleteResult struct {
	ActivityID       string
	ProjectID        string
	RecipientInboxes []string
}

// MembershipResult carries the ActivityPub side effects produced by membership changes.
type MembershipResult struct {
	ActivityID       string
	ProjectID        string
	RecipientInboxes []string
}

// IsValidRole reports whether role is a supported project role.
func IsValidRole(role string) bool {
	switch role {
	case RoleOwner, RoleManager, RoleDeveloper, RoleViewer:
		return true
	default:
		return false
	}
}

// CanManageProject reports whether role can edit project metadata.
func CanManageProject(role string) bool {
	return role == RoleOwner || role == RoleManager
}

// CanDeleteProject reports whether role can delete a project.
func CanDeleteProject(role string) bool {
	return role == RoleOwner
}

// CanManageMembers reports whether role can invite or remove project members.
func CanManageMembers(role string) bool {
	return role == RoleOwner || role == RoleManager
}

// CanWriteTickets reports whether role can create and edit tickets or comments.
func CanWriteTickets(role string) bool {
	return role == RoleOwner || role == RoleManager || role == RoleDeveloper
}

// CanDeleteTickets reports whether role can delete tickets.
func CanDeleteTickets(role string) bool {
	return role == RoleOwner || role == RoleManager
}

// CanModerateComments reports whether role can delete comments written by others.
func CanModerateComments(role string) bool {
	return role == RoleOwner || role == RoleManager
}
