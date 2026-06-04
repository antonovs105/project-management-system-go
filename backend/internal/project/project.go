package project

import "time"

const (
	// RoleOwner is the default full-control project role key.
	RoleOwner = "owner"
	// RoleManager is the default project-management role key.
	RoleManager = "manager"
	// RoleDeveloper is the default ticket-work role key.
	RoleDeveloper = "developer"
	// RoleViewer is the default read-only project role key.
	RoleViewer = "viewer"
)

const (
	// PermissionProjectRead allows reading project resources.
	PermissionProjectRead = "project.read"
	// PermissionProjectUpdate allows updating project metadata.
	PermissionProjectUpdate = "project.update"
	// PermissionProjectDelete allows deleting a project.
	PermissionProjectDelete = "project.delete"
	// PermissionMembersInvite allows inviting new project members.
	PermissionMembersInvite = "members.invite"
	// PermissionMembersRemove allows removing project members.
	PermissionMembersRemove = "members.remove"
	// PermissionRolesManage allows creating and changing project roles.
	PermissionRolesManage = "roles.manage"
	// PermissionTicketsCreate allows creating tickets.
	PermissionTicketsCreate = "tickets.create"
	// PermissionTicketsUpdate allows updating tickets and ticket links.
	PermissionTicketsUpdate = "tickets.update"
	// PermissionTicketsDelete allows deleting tickets.
	PermissionTicketsDelete = "tickets.delete"
	// PermissionCommentsCreate allows creating comments and deleting your own comments.
	PermissionCommentsCreate = "comments.create"
	// PermissionCommentsModerate allows deleting comments by other people.
	PermissionCommentsModerate = "comments.moderate"
	// PermissionFederationDeliveryRetry allows retrying failed project delivery jobs.
	PermissionFederationDeliveryRetry = "federation.delivery.retry"
)

// SupportedProjectPermissions is the allow-list for configurable project permissions.
var SupportedProjectPermissions = []string{
	PermissionProjectRead,
	PermissionProjectUpdate,
	PermissionProjectDelete,
	PermissionMembersInvite,
	PermissionMembersRemove,
	PermissionRolesManage,
	PermissionTicketsCreate,
	PermissionTicketsUpdate,
	PermissionTicketsDelete,
	PermissionCommentsCreate,
	PermissionCommentsModerate,
	PermissionFederationDeliveryRetry,
}

// DefaultProjectRoles seeds the initial permission model for each new project.
var DefaultProjectRoles = []ProjectRole{
	{
		Key:         RoleOwner,
		Name:        "Owner",
		Description: "Full project control, including roles and destructive actions.",
		IsSystem:    true,
		Position:    10,
		Permissions: []string{
			PermissionProjectRead,
			PermissionProjectUpdate,
			PermissionProjectDelete,
			PermissionMembersInvite,
			PermissionMembersRemove,
			PermissionRolesManage,
			PermissionTicketsCreate,
			PermissionTicketsUpdate,
			PermissionTicketsDelete,
			PermissionCommentsCreate,
			PermissionCommentsModerate,
			PermissionFederationDeliveryRetry,
		},
	},
	{
		Key:         RoleManager,
		Name:        "Manager",
		Description: "Can manage project work and membership without deleting the project.",
		Position:    20,
		Permissions: []string{
			PermissionProjectRead,
			PermissionProjectUpdate,
			PermissionMembersInvite,
			PermissionMembersRemove,
			PermissionTicketsCreate,
			PermissionTicketsUpdate,
			PermissionTicketsDelete,
			PermissionCommentsCreate,
			PermissionCommentsModerate,
			PermissionFederationDeliveryRetry,
		},
	},
	{
		Key:         RoleDeveloper,
		Name:        "Developer",
		Description: "Can work with tickets and comments.",
		Position:    30,
		Permissions: []string{
			PermissionProjectRead,
			PermissionTicketsCreate,
			PermissionTicketsUpdate,
			PermissionCommentsCreate,
		},
	},
	{
		Key:         RoleViewer,
		Name:        "Viewer",
		Description: "Can read project content only.",
		Position:    40,
		Permissions: []string{
			PermissionProjectRead,
		},
	},
}

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
	RoleID         string    `db:"role_id" json:"role_id"`
	Role           string    `db:"role" json:"role"`
	Status         string    `db:"status" json:"status"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}

// ProjectInviteInspection is an operator-facing invite view with actor and role labels.
type ProjectInviteInspection struct {
	ID              string    `db:"id" json:"id"`
	APID            string    `db:"ap_id" json:"ap_id"`
	ProjectID       string    `db:"project_id" json:"project_id"`
	InviterActorID  string    `db:"inviter_actor_id" json:"inviter_actor_id"`
	InviteeActorID  string    `db:"invitee_actor_id" json:"invitee_actor_id"`
	RoleID          string    `db:"role_id" json:"role_id"`
	Role            string    `db:"role" json:"role"`
	RoleName        string    `db:"role_name" json:"role_name"`
	Status          string    `db:"status" json:"status"`
	InviterUsername string    `db:"inviter_username" json:"inviter_username"`
	InviterEmail    string    `db:"inviter_email" json:"inviter_email"`
	InviterHandle   string    `db:"inviter_handle" json:"inviter_handle"`
	InviterName     string    `db:"inviter_name" json:"inviter_name"`
	InviteeUsername string    `db:"invitee_username" json:"invitee_username"`
	InviteeEmail    string    `db:"invitee_email" json:"invitee_email"`
	InviteeHandle   string    `db:"invitee_handle" json:"invitee_handle"`
	InviteeName     string    `db:"invitee_name" json:"invitee_name"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time `db:"updated_at" json:"updated_at"`
}

// ProjectMember is the relational projection of a user's role in a project.
type ProjectMember struct {
	UserID    string    `db:"user_id" json:"user_id"`
	ProjectID string    `db:"project_id" json:"project_id"`
	RoleID    string    `db:"role_id" json:"role_id"`
	Role      string    `db:"role" json:"role"`
	RoleName  string    `db:"role_name" json:"role_name"`
	Username  string    `db:"username" json:"username"`
	Email     string    `db:"email" json:"email"`
	Handle    string    `db:"handle" json:"handle"`
	Name      string    `db:"name" json:"name"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// ProjectRole is a configurable project-local role with explicit permissions.
type ProjectRole struct {
	ID          string    `db:"id" json:"id"`
	ProjectID   string    `db:"project_id" json:"project_id"`
	Key         string    `db:"key" json:"key"`
	Name        string    `db:"name" json:"name"`
	Description string    `db:"description" json:"description"`
	IsSystem    bool      `db:"is_system" json:"is_system"`
	Position    int       `db:"position" json:"position"`
	Permissions []string  `db:"-" json:"permissions"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// ProjectListOptions contains pagination for project list responses.
type ProjectListOptions struct {
	Limit  int
	Offset int
}

// ProjectInviteListOptions contains filters and pagination for invite inspection.
type ProjectInviteListOptions struct {
	Status string
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

// IsSupportedPermission reports whether permission can be assigned to a project role.
func IsSupportedPermission(permission string) bool {
	for _, supported := range SupportedProjectPermissions {
		if permission == supported {
			return true
		}
	}
	return false
}

// hasPermission reports whether a permission list grants permission.
func hasPermission(permissions []string, permission string) bool {
	for _, candidate := range permissions {
		if candidate == permission {
			return true
		}
	}
	return false
}
