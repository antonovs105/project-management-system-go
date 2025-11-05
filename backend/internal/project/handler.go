package project

import (
	"log"
	"net/http"
	"strconv" // Для конвертации строки в число

	"github.com/labstack/echo/v4"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
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
	userID := c.Get("userID").(int64)

	project, err := h.service.CreateProject(c.Request().Context(), req.Name, req.Description, userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create project"})
	}

	return c.JSON(http.StatusCreated, project)
}

// Get handler for GET /api/projects/:id
func (h *Handler) Get(c echo.Context) error {
	projectIDStr := c.Param("id")
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid project ID"})
	}

	userID := c.Get("userID").(int64)

	project, err := h.service.GetProjectByID(c.Request().Context(), projectID, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, project)
}

// List handler of GET /api/projects
func (h *Handler) List(c echo.Context) error {
	userID := c.Get("userID").(int64)

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
	// get project id
	projectIDStr := c.Param("id")
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid project ID"})
	}

	// parsing request body
	var req UpdateProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	userID := c.Get("userID").(int64)

	// call service for update
	err = h.service.UpdateProject(c.Request().Context(), projectID, userID, req)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// Delete handler of DELETE /api/projects/:id
func (h *Handler) Delete(c echo.Context) error {
	// get project id (i am repeating myself)
	projectIDStr := c.Param("id")
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid project ID"})
	}

	userID := c.Get("userID").(int64)

	// call service for deleting
	err = h.service.DeleteProject(c.Request().Context(), projectID, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// addMemberRequest struct for parsing JSON request
type addMemberRequest struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
}

// AddMember handler for POST /api/projects/:id/members
func (h *Handler) AddMember(c echo.Context) error {
	projectID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid project ID"})
	}

	currentUserID := c.Get("userID").(int64)

	// parse request
	var req addMemberRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	// call service logic
	err = h.service.AddMemberToProject(c.Request().Context(), projectID, currentUserID, req.UserID, req.Role)
	if err != nil {
		// TODO: add more clarity errors
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
