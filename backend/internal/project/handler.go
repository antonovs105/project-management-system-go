package project

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

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

type createProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Create is handler of POST /api/projects
func (h *Handler) Create(c echo.Context) error {
	var req createProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	// Taking userID from context
	userID := c.Get("userID").(string)

	project, err := h.service.CreateProject(c.Request().Context(), req.Name, req.Description, userID)
	if err != nil {
		return writeProjectError(c, err)
	}

	return c.JSON(http.StatusCreated, project)
}

// Get handler for GET /api/projects/:id
func (h *Handler) Get(c echo.Context) error {
	projectID := c.Param("id")

	userID := c.Get("userID").(string)

	project, err := h.service.GetProjectByID(c.Request().Context(), projectID, userID)
	if err != nil {
		return writeProjectError(c, err)
	}

	return c.JSON(http.StatusOK, project)
}

// List handler of GET /api/projects
func (h *Handler) List(c echo.Context) error {
	userID := c.Get("userID").(string)

	// Call service for projects list
	projects, err := h.service.ListUserProjects(c.Request().Context(), userID)
	if err != nil {
		log.Printf("Error listing user projects: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve projects"})
	}

	return c.JSON(http.StatusOK, projects)
}

// Update handler for PATCH /api/projects/:id
func (h *Handler) Update(c echo.Context) error {
	projectID := c.Param("id")

	// parsing request body
	var req UpdateProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	userID := c.Get("userID").(string)

	// call service for update
	if err := h.service.UpdateProject(c.Request().Context(), projectID, userID, req); err != nil {
		return writeProjectError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// Delete handler of DELETE /api/projects/:id
func (h *Handler) Delete(c echo.Context) error {
	projectID := c.Param("id")

	userID := c.Get("userID").(string)

	// call service for deleting
	if err := h.service.DeleteProject(c.Request().Context(), projectID, userID); err != nil {
		return writeProjectError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// addMemberRequest struct for parsing JSON request
type addMemberRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// AddMember handler for POST /api/projects/:id/members
func (h *Handler) AddMember(c echo.Context) error {
	projectID := c.Param("id")

	currentUserID := c.Get("userID").(string)

	// parse request
	var req addMemberRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	// call service logic
	invite, err := h.service.AddMemberToProject(c.Request().Context(), projectID, currentUserID, req.UserID, req.Role)
	if err != nil {
		return writeProjectError(c, err)
	}

	return c.JSON(http.StatusAccepted, invite)
}

func (h *Handler) RemoveMember(c echo.Context) error {
	projectID := c.Param("id")
	targetUserID := c.Param("userID")
	currentUserID := c.Get("userID").(string)

	if err := h.service.RemoveMemberFromProject(c.Request().Context(), projectID, currentUserID, targetUserID); err != nil {
		return writeProjectError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// AcceptInvite handler for POST /api/invites/:id/accept
func (h *Handler) AcceptInvite(c echo.Context) error {
	inviteID := c.Param("id")
	userID := c.Get("userID").(string)

	if err := h.service.AcceptInvite(c.Request().Context(), inviteID, userID); err != nil {
		return writeProjectError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) RejectInvite(c echo.Context) error {
	inviteID := c.Param("id")
	userID := c.Get("userID").(string)

	if err := h.service.RejectInvite(c.Request().Context(), inviteID, userID); err != nil {
		return writeProjectError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) RevokeInvite(c echo.Context) error {
	inviteID := c.Param("id")
	userID := c.Get("userID").(string)

	if err := h.service.RevokeInvite(c.Request().Context(), inviteID, userID); err != nil {
		return writeProjectError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

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
