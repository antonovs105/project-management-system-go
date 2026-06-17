package githubintegration

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// Handler exposes GitHub integration HTTP endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates a GitHub integration HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers authenticated GitHub integration routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/projects/:projectID/github/repositories", h.ListRepositories)
	api.POST("/projects/:projectID/github/repositories", h.LinkRepository)
	api.DELETE("/projects/:projectID/github/repositories/:repositoryID", h.DeleteRepository)
	api.POST("/projects/:projectID/github/repositories/:repositoryID/sync", h.SyncRepository)
	api.GET("/tickets/:ticketID/github/commits", h.ListTicketCommits)
}

// linkRepositoryRequest is the JSON payload for attaching a GitHub repository.
type linkRepositoryRequest struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

// ListRepositories returns GitHub repositories attached to a project.
func (h *Handler) ListRepositories(c echo.Context) error {
	repos, err := h.service.ListRepositories(c.Request().Context(), c.Param("projectID"), currentUserID(c))
	if err != nil {
		return writeGitHubError(c, err)
	}
	return c.JSON(http.StatusOK, repos)
}

// LinkRepository attaches a GitHub repository to a project.
func (h *Handler) LinkRepository(c echo.Context) error {
	var req linkRepositoryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	repo, err := h.service.LinkRepository(c.Request().Context(), c.Param("projectID"), currentUserID(c), req.Owner, req.Name)
	if err != nil {
		return writeGitHubError(c, err)
	}
	return c.JSON(http.StatusCreated, repo)
}

// DeleteRepository removes a project GitHub repository link.
func (h *Handler) DeleteRepository(c echo.Context) error {
	if err := h.service.DeleteRepository(c.Request().Context(), c.Param("projectID"), currentUserID(c), c.Param("repositoryID")); err != nil {
		return writeGitHubError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// SyncRepository imports recent commits for a project GitHub repository.
func (h *Handler) SyncRepository(c echo.Context) error {
	result, err := h.service.SyncRepository(c.Request().Context(), c.Param("projectID"), currentUserID(c), c.Param("repositoryID"))
	if err != nil {
		return writeGitHubError(c, err)
	}
	return c.JSON(http.StatusAccepted, result)
}

// ListTicketCommits returns imported GitHub commits linked to one ticket.
func (h *Handler) ListTicketCommits(c echo.Context) error {
	commits, err := h.service.ListTicketCommits(c.Request().Context(), c.Param("ticketID"), currentUserID(c))
	if err != nil {
		return writeGitHubError(c, err)
	}
	return c.JSON(http.StatusOK, commits)
}

// currentUserID returns the authenticated user ID stored by JWT middleware.
func currentUserID(c echo.Context) string {
	userID, _ := c.Get("userID").(string)
	return userID
}

// writeGitHubError maps service errors to HTTP responses.
func writeGitHubError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrGitHubNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrGitHubRequestFailed):
		return c.JSON(http.StatusBadGateway, map[string]string{"error": err.Error()})
	case strings.Contains(err.Error(), "insufficient permissions"):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	case strings.Contains(err.Error(), "not found or access denied"):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		log.Printf("github integration request failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "github integration request failed"})
	}
}
