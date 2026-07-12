package project

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/apiresponse"
	"github.com/antonovs105/project-management-system-go/internal/apperror"
	appconfig "github.com/antonovs105/project-management-system-go/internal/config"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Handler exposes project HTTP endpoints.
type Handler struct {
	service               *Service
	instanceRoles         InstanceRoleProvider
	projectCreationPolicy string
}

// InstanceRoleProvider resolves system-wide roles for instance policy checks.
type InstanceRoleProvider interface {
	InstanceRole(ctx context.Context, userID string) (string, error)
}

// HandlerOption customizes project HTTP behavior.
type HandlerOption func(*Handler)

// WithInstanceRoleProvider supplies instance role lookup for global policies.
func WithInstanceRoleProvider(provider InstanceRoleProvider) HandlerOption {
	return func(h *Handler) {
		h.instanceRoles = provider
	}
}

// WithProjectCreationPolicy controls which local users may create projects.
func WithProjectCreationPolicy(policy string) HandlerOption {
	return func(h *Handler) {
		policy = strings.TrimSpace(policy)
		if policy != "" {
			h.projectCreationPolicy = policy
		}
	}
}

// NewHandler creates a project HTTP handler.
func NewHandler(service *Service, options ...HandlerOption) *Handler {
	handler := &Handler{
		service:               service,
		projectCreationPolicy: appconfig.ProjectCreationEveryone,
	}
	for _, option := range options {
		option(handler)
	}
	return handler
}

// RegisterRoutes registers authenticated project routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/me/invites", h.ListMyInvites)
	api.POST("/projects", h.Create)
	api.GET("/projects/:id", h.Get)
	api.GET("/projects", h.List)
	api.PATCH("/projects/:id", h.Update)
	api.DELETE("/projects/:id", h.Delete)
	api.GET("/projects/:id/roles", h.ListRoles)
	api.POST("/projects/:id/roles", h.CreateRole)
	api.PATCH("/projects/:id/roles/:roleID", h.UpdateRole)
	api.DELETE("/projects/:id/roles/:roleID", h.DeleteRole)
	api.GET("/projects/:id/members", h.ListMembers)
	api.POST("/projects/:id/members", h.AddMember)
	api.PATCH("/projects/:id/members/:userID", h.UpdateMemberRole)
	api.DELETE("/projects/:id/members/:userID", h.RemoveMember)
	api.GET("/projects/:id/invites", h.ListInvites)
	api.POST("/invites/:id/accept", h.AcceptInvite)
	api.POST("/invites/:id/reject", h.RejectInvite)
	api.POST("/invites/:id/revoke", h.RevokeInvite)
}

// createProjectRequest is the JSON payload for creating a project.
type createProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Create creates a project owned by the current user.
func (h *Handler) Create(c echo.Context) error {
	var req createProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	userID := c.Get("userID").(string)
	if err := h.requireProjectCreationAllowed(c.Request().Context(), userID); err != nil {
		return writeProjectError(c, err)
	}

	project, err := h.service.CreateProject(c.Request().Context(), req.Name, req.Description, userID)
	if err != nil {
		return writeProjectError(c, err)
	}

	return c.JSON(http.StatusCreated, project)
}

// requireProjectCreationAllowed enforces instance-wide project creation policy.
func (h *Handler) requireProjectCreationAllowed(ctx context.Context, userID string) error {
	switch h.projectCreationPolicy {
	case "", appconfig.ProjectCreationEveryone:
		return nil
	case appconfig.ProjectCreationAdminsOnly:
		if h.instanceRoles == nil {
			return errors.New("project creation policy is missing instance role provider")
		}
		role, err := h.instanceRoles.InstanceRole(ctx, userID)
		if err != nil {
			return err
		}
		if !user.HasAdminPrivileges(role) {
			return errors.New("insufficient permissions: project creation is restricted to instance administrators")
		}
		return nil
	default:
		return errors.New("invalid project creation policy")
	}
}

// Get returns a project visible to the current user.
func (h *Handler) Get(c echo.Context) error {
	projectID, ok := uuidParam(c, "id", "project id")
	if !ok {
		return nil
	}

	userID := c.Get("userID").(string)

	project, err := h.service.GetProjectByID(c.Request().Context(), projectID, userID)
	if err != nil {
		return writeProjectError(c, err)
	}
	apiresponse.SetVersionETag(c, project.Version)
	return c.JSON(http.StatusOK, project)
}

// List returns projects where the current user is a member.
func (h *Handler) List(c echo.Context) error {
	userID := c.Get("userID").(string)
	options, err := projectListOptions(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	projects, err := h.service.ListUserProjects(c.Request().Context(), userID, options)
	if err != nil {
		log.Printf("Error listing user projects: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to retrieve projects"})
	}

	return apiresponse.WriteOffsetPage(c, http.StatusOK, projects, options.Limit, options.Offset)
}

// ListMyInvites returns invites addressed to the current user.
func (h *Handler) ListMyInvites(c echo.Context) error {
	userID := c.Get("userID").(string)
	options, err := projectInviteListOptions(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	invites, err := h.service.ListUserInvites(c.Request().Context(), userID, options)
	if err != nil {
		return writeProjectError(c, err)
	}
	return apiresponse.WriteOffsetPage(c, http.StatusOK, invites, options.Limit, options.Offset)
}

// Update changes project metadata.
func (h *Handler) Update(c echo.Context) error {
	projectID, ok := uuidParam(c, "id", "project id")
	if !ok {
		return nil
	}

	var req UpdateProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	version, err := apiresponse.ExpectedVersion(c)
	if err != nil {
		return c.JSON(http.StatusPreconditionRequired, map[string]string{"error": err.Error(), "code": "if_match_required"})
	}
	req.ExpectedVersion = version

	userID := c.Get("userID").(string)

	if err := h.service.UpdateProject(c.Request().Context(), projectID, userID, req); err != nil {
		return writeProjectError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// Delete removes a project.
func (h *Handler) Delete(c echo.Context) error {
	projectID, ok := uuidParam(c, "id", "project id")
	if !ok {
		return nil
	}

	userID := c.Get("userID").(string)

	if err := h.service.DeleteProject(c.Request().Context(), projectID, userID); err != nil {
		return writeProjectError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// addMemberRequest is the JSON payload for inviting a project member.
type addMemberRequest struct {
	UserID  string `json:"user_id"`
	UserRef string `json:"user_ref"`
	Role    string `json:"role"`
	RoleID  string `json:"role_id"`
}

// AddMember creates an invite for a new project member.
func (h *Handler) AddMember(c echo.Context) error {
	projectID, ok := uuidParam(c, "id", "project id")
	if !ok {
		return nil
	}

	currentUserID := c.Get("userID").(string)

	var req addMemberRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.UserID != "" && !validUUID(req.UserID) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid user id"})
	}
	inviteeRef := strings.TrimSpace(req.UserRef)
	if inviteeRef == "" {
		inviteeRef = strings.TrimSpace(req.UserID)
	}
	if inviteeRef == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invitee is required"})
	}

	roleRef := req.RoleID
	if roleRef == "" {
		roleRef = req.Role
	}
	invite, err := h.service.AddMemberToProject(c.Request().Context(), projectID, currentUserID, inviteeRef, roleRef)
	if err != nil {
		return writeProjectError(c, err)
	}

	return c.JSON(http.StatusAccepted, invite)
}

// ListMembers returns project members for membership managers.
func (h *Handler) ListMembers(c echo.Context) error {
	projectID, ok := uuidParam(c, "id", "project id")
	if !ok {
		return nil
	}
	options, err := projectListOptions(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	userID := c.Get("userID").(string)
	members, err := h.service.ListProjectMembers(c.Request().Context(), projectID, userID, options)
	if err != nil {
		return writeProjectError(c, err)
	}
	return apiresponse.WriteOffsetPage(c, http.StatusOK, members, options.Limit, options.Offset)
}

// ListInvites returns project invite history for membership managers.
func (h *Handler) ListInvites(c echo.Context) error {
	projectID, ok := uuidParam(c, "id", "project id")
	if !ok {
		return nil
	}
	options, err := projectInviteListOptions(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	userID := c.Get("userID").(string)
	invites, err := h.service.ListProjectInvites(c.Request().Context(), projectID, userID, options)
	if err != nil {
		return writeProjectError(c, err)
	}
	return apiresponse.WriteOffsetPage(c, http.StatusOK, invites, options.Limit, options.Offset)
}

// ListRoles returns configurable project roles.
func (h *Handler) ListRoles(c echo.Context) error {
	projectID, ok := uuidParam(c, "id", "project id")
	if !ok {
		return nil
	}
	userID := c.Get("userID").(string)
	roles, err := h.service.ListProjectRoles(c.Request().Context(), projectID, userID)
	if err != nil {
		return writeProjectError(c, err)
	}
	return c.JSON(http.StatusOK, roles)
}

// CreateRole creates a project role.
func (h *Handler) CreateRole(c echo.Context) error {
	projectID, ok := uuidParam(c, "id", "project id")
	if !ok {
		return nil
	}
	var req CreateProjectRoleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	userID := c.Get("userID").(string)
	role, err := h.service.CreateProjectRole(c.Request().Context(), projectID, userID, req)
	if err != nil {
		return writeProjectError(c, err)
	}
	return c.JSON(http.StatusCreated, role)
}

// UpdateRole updates a project role.
func (h *Handler) UpdateRole(c echo.Context) error {
	projectID, ok := uuidParam(c, "id", "project id")
	if !ok {
		return nil
	}
	roleID, ok := uuidParam(c, "roleID", "role id")
	if !ok {
		return nil
	}
	var req UpdateProjectRoleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	userID := c.Get("userID").(string)
	role, err := h.service.UpdateProjectRole(c.Request().Context(), projectID, userID, roleID, req)
	if err != nil {
		return writeProjectError(c, err)
	}
	return c.JSON(http.StatusOK, role)
}

// DeleteRole removes an unused project role.
func (h *Handler) DeleteRole(c echo.Context) error {
	projectID, ok := uuidParam(c, "id", "project id")
	if !ok {
		return nil
	}
	roleID, ok := uuidParam(c, "roleID", "role id")
	if !ok {
		return nil
	}
	userID := c.Get("userID").(string)
	if err := h.service.DeleteProjectRole(c.Request().Context(), projectID, userID, roleID); err != nil {
		return writeProjectError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// updateMemberRoleRequest is the JSON payload for changing a project member's role.
type updateMemberRoleRequest struct {
	RoleID string `json:"role_id"`
	Role   string `json:"role"`
}

// UpdateMemberRole changes an existing project member role.
func (h *Handler) UpdateMemberRole(c echo.Context) error {
	projectID, ok := uuidParam(c, "id", "project id")
	if !ok {
		return nil
	}
	targetUserID, ok := uuidParam(c, "userID", "user id")
	if !ok {
		return nil
	}
	var req updateMemberRoleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	roleRef := req.RoleID
	if roleRef == "" {
		roleRef = req.Role
	}
	userID := c.Get("userID").(string)
	member, err := h.service.UpdateProjectMemberRole(c.Request().Context(), projectID, userID, targetUserID, roleRef)
	if err != nil {
		return writeProjectError(c, err)
	}
	return c.JSON(http.StatusOK, member)
}

// RemoveMember removes a user from a project.
func (h *Handler) RemoveMember(c echo.Context) error {
	projectID, ok := uuidParam(c, "id", "project id")
	if !ok {
		return nil
	}
	targetUserID, ok := uuidParam(c, "userID", "user id")
	if !ok {
		return nil
	}
	currentUserID := c.Get("userID").(string)

	if err := h.service.RemoveMemberFromProject(c.Request().Context(), projectID, currentUserID, targetUserID); err != nil {
		return writeProjectError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// AcceptInvite accepts a pending invite for the current user.
func (h *Handler) AcceptInvite(c echo.Context) error {
	inviteID, ok := uuidParam(c, "id", "invite id")
	if !ok {
		return nil
	}
	userID := c.Get("userID").(string)

	if err := h.service.AcceptInvite(c.Request().Context(), inviteID, userID); err != nil {
		return writeProjectError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// RejectInvite rejects a pending invite for the current user.
func (h *Handler) RejectInvite(c echo.Context) error {
	inviteID, ok := uuidParam(c, "id", "invite id")
	if !ok {
		return nil
	}
	userID := c.Get("userID").(string)

	if err := h.service.RejectInvite(c.Request().Context(), inviteID, userID); err != nil {
		return writeProjectError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// RevokeInvite revokes a pending invite.
func (h *Handler) RevokeInvite(c echo.Context) error {
	inviteID, ok := uuidParam(c, "id", "invite id")
	if !ok {
		return nil
	}
	userID := c.Get("userID").(string)

	if err := h.service.RevokeInvite(c.Request().Context(), inviteID, userID); err != nil {
		return writeProjectError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// projectListOptions parses bounded project list pagination.
func projectListOptions(c echo.Context) (ProjectListOptions, error) {
	limit, err := parseOptionalPositiveInt(c.QueryParam("limit"))
	if err != nil {
		return ProjectListOptions{}, ErrInvalidProjectInput
	}
	offset, err := parseOptionalNonNegativeInt(c.QueryParam("offset"))
	if err != nil {
		return ProjectListOptions{}, ErrInvalidProjectInput
	}
	return ProjectListOptions{Limit: limit, Offset: offset}, nil
}

// projectInviteListOptions parses invite filters and pagination.
func projectInviteListOptions(c echo.Context) (ProjectInviteListOptions, error) {
	options, err := projectListOptions(c)
	if err != nil {
		return ProjectInviteListOptions{}, err
	}
	return ProjectInviteListOptions{
		Status: strings.TrimSpace(c.QueryParam("status")),
		Limit:  options.Limit,
		Offset: options.Offset,
	}, nil
}

// parseOptionalPositiveInt parses an optional positive pagination value.
func parseOptionalPositiveInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, ErrInvalidProjectInput
	}
	return value, nil
}

// parseOptionalNonNegativeInt parses an optional non-negative pagination value.
func parseOptionalNonNegativeInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, ErrInvalidProjectInput
	}
	return value, nil
}

// uuidParam extracts and validates a UUID path parameter.
func uuidParam(c echo.Context, name string, label string) (string, bool) {
	value := c.Param(name)
	if !validUUID(value) {
		_ = c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid " + label})
		return "", false
	}
	return value, true
}

// validUUID reports whether value is a valid UUID.
func validUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

// writeProjectError maps project service errors to HTTP responses.
func writeProjectError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrInvalidProjectInput):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrConflict):
		return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrPrecondition):
		return c.JSON(http.StatusPreconditionFailed, map[string]string{"error": err.Error(), "code": "version_conflict"})
	case errors.Is(err, apperror.ErrForbidden):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "project operation failed"})
	}
}
