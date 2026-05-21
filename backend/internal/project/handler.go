package project

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Handler exposes project HTTP endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates a project HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers authenticated project routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.POST("/projects", h.Create)
	api.GET("/projects/:id", h.Get)
	api.GET("/projects", h.List)
	api.PATCH("/projects/:id", h.Update)
	api.DELETE("/projects/:id", h.Delete)
	api.POST("/projects/:id/members", h.AddMember)
	api.DELETE("/projects/:id/members/:userID", h.RemoveMember)
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

	project, err := h.service.CreateProject(c.Request().Context(), req.Name, req.Description, userID)
	if err != nil {
		return writeProjectError(c, err)
	}

	return c.JSON(http.StatusCreated, project)
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

	return c.JSON(http.StatusOK, projects)
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
	UserID string `json:"user_id"`
	Role   string `json:"role"`
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
	if !validUUID(req.UserID) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid user id"})
	}

	invite, err := h.service.AddMemberToProject(c.Request().Context(), projectID, currentUserID, req.UserID, req.Role)
	if err != nil {
		return writeProjectError(c, err)
	}

	return c.JSON(http.StatusAccepted, invite)
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
	case strings.Contains(err.Error(), "already"), strings.Contains(err.Error(), "not pending"):
		return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
	case strings.Contains(err.Error(), "insufficient permissions"):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	case strings.Contains(err.Error(), "access denied"), strings.Contains(err.Error(), "not found"):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "project operation failed"})
	}
}
