package project

import (
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
