package project

import "time"

const (
	RoleOwner     = "owner"
	RoleManager   = "manager"
	RoleDeveloper = "developer"
	RoleViewer    = "viewer"
)

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

type ProjectMember struct {
	UserID    string    `db:"user_id" json:"user_id"`
	ProjectID string    `db:"project_id" json:"project_id"`
	Role      string    `db:"role" json:"role"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type UpdateResult struct {
	ActivityID       string
	ProjectID        string
	RecipientInboxes []string
}

type DeleteResult struct {
	ActivityID       string
	ProjectID        string
	RecipientInboxes []string
}

type MembershipResult struct {
	ActivityID       string
	ProjectID        string
	RecipientInboxes []string
}

func IsValidRole(role string) bool {
	switch role {
	case RoleOwner, RoleManager, RoleDeveloper, RoleViewer:
		return true
	default:
		return false
	}
}

func CanManageProject(role string) bool {
	return role == RoleOwner || role == RoleManager
}

func CanDeleteProject(role string) bool {
	return role == RoleOwner
}

func CanManageMembers(role string) bool {
	return role == RoleOwner || role == RoleManager
}

func CanWriteTickets(role string) bool {
	return role == RoleOwner || role == RoleManager || role == RoleDeveloper
}

func CanDeleteTickets(role string) bool {
	return role == RoleOwner || role == RoleManager
}

func CanModerateComments(role string) bool {
	return role == RoleOwner || role == RoleManager
}
