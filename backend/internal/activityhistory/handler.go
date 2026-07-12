package activityhistory

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// Handler exposes project activity history.
type Handler struct{ service *Service }

// NewHandler returns an activity history handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// RegisterRoutes mounts activity history under the authenticated API.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/projects/:projectID/activity", h.List)
	api.GET("/projects/archived", h.ListArchivedProjects)
	api.GET("/projects/:projectID/tickets/archived", h.ListArchivedTickets)
	api.POST("/projects/:projectID/archive", h.ArchiveProject)
	api.POST("/projects/:projectID/restore", h.RestoreProject)
	api.POST("/tickets/:ticketID/archive", h.ArchiveTicket)
	api.POST("/tickets/:ticketID/restore", h.RestoreTicket)
}

// List returns a bounded project activity page.
func (h *Handler) List(c echo.Context) error {
	limit, err := strconv.Atoi(c.QueryParam("limit"))
	if c.QueryParam("limit") != "" && err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid limit"})
	}
	offset, err := strconv.Atoi(c.QueryParam("offset"))
	if c.QueryParam("offset") != "" && err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid offset"})
	}
	userID, _ := c.Get("userID").(string)
	values, err := h.service.List(c.Request().Context(), c.Param("projectID"), userID, limit, offset)
	if errors.Is(err, ErrForbidden) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "activity history failed"})
	}
	return c.JSON(http.StatusOK, values)
}

// ListArchivedProjects returns restorable projects for the current user.
func (h *Handler) ListArchivedProjects(c echo.Context) error {
	userID, _ := c.Get("userID").(string)
	values, err := h.service.ListArchivedProjects(c.Request().Context(), userID)
	if err != nil {
		return writeActivityError(c, err)
	}
	return c.JSON(http.StatusOK, values)
}

// ListArchivedTickets returns restorable tickets in one project.
func (h *Handler) ListArchivedTickets(c echo.Context) error {
	userID, _ := c.Get("userID").(string)
	values, err := h.service.ListArchivedTickets(c.Request().Context(), c.Param("projectID"), userID)
	if err != nil {
		return writeActivityError(c, err)
	}
	return c.JSON(http.StatusOK, values)
}

// ArchiveProject soft-deletes a project using its expected version.
func (h *Handler) ArchiveProject(c echo.Context) error { return h.setProjectArchived(c, true) }

// RestoreProject restores a project using its expected version.
func (h *Handler) RestoreProject(c echo.Context) error { return h.setProjectArchived(c, false) }

// ArchiveTicket soft-deletes a ticket using its expected version.
func (h *Handler) ArchiveTicket(c echo.Context) error { return h.setTicketArchived(c, true) }

// RestoreTicket restores a ticket using its expected version.
func (h *Handler) RestoreTicket(c echo.Context) error { return h.setTicketArchived(c, false) }

// setProjectArchived executes one project archive transition.
func (h *Handler) setProjectArchived(c echo.Context, archived bool) error {
	version, err := expectedVersion(c)
	if err != nil {
		return c.JSON(http.StatusPreconditionRequired, map[string]string{"error": err.Error(), "code": "if_match_required"})
	}
	userID, _ := c.Get("userID").(string)
	err = h.service.SetProjectArchived(c.Request().Context(), c.Param("projectID"), userID, version, archived)
	if err != nil {
		return writeActivityError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// setTicketArchived executes one ticket archive transition.
func (h *Handler) setTicketArchived(c echo.Context, archived bool) error {
	version, err := expectedVersion(c)
	if err != nil {
		return c.JSON(http.StatusPreconditionRequired, map[string]string{"error": err.Error(), "code": "if_match_required"})
	}
	userID, _ := c.Get("userID").(string)
	err = h.service.SetTicketArchived(c.Request().Context(), c.Param("ticketID"), userID, version, archived)
	if err != nil {
		return writeActivityError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// expectedVersion parses a strong numeric If-Match entity tag.
func expectedVersion(c echo.Context) (int64, error) {
	value := strings.TrimSpace(c.Request().Header.Get("If-Match"))
	if strings.HasPrefix(value, "W/") {
		return 0, errors.New("a strong If-Match version is required")
	}
	value = strings.Trim(value, `"`)
	version, err := strconv.ParseInt(value, 10, 64)
	if err != nil || version < 1 {
		return 0, errors.New("a valid If-Match version is required")
	}
	return version, nil
}

// writeActivityError maps archive/history service errors.
func writeActivityError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrForbidden):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrVersionConflict):
		return c.JSON(http.StatusPreconditionFailed, map[string]string{"error": err.Error(), "code": "version_conflict"})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "activity operation failed"})
	}
}
