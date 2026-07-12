package portability

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/apperror"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

const maxImportBodyBytes = 10 << 20

var safeDownloadName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// Handler exposes authenticated portability endpoints.
type Handler struct{ service *Service }

// NewHandler creates a portability handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// RegisterRoutes mounts project and account export/import routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/projects/:projectID/export", h.ExportProject)
	api.POST("/projects/import", h.ImportProject)
	api.POST("/projects/:projectID/tickets/import", h.ImportTickets)
	api.GET("/me/export", h.ExportUser)
}

// ExportProject downloads a versioned project bundle.
func (h *Handler) ExportProject(c echo.Context) error {
	projectID, ok := portabilityUUID(c, "projectID")
	if !ok {
		return nil
	}
	bundle, err := h.service.ExportProject(c.Request().Context(), projectID, c.Get("userID").(string))
	if err != nil {
		return writePortabilityError(c, err)
	}
	name := safeDownloadName.ReplaceAllString(strings.TrimSpace(bundle.Project.Name), "-")
	if name == "" {
		name = "project"
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="%s.progo.json"`, name))
	return c.JSON(http.StatusOK, bundle)
}

// ExportUser downloads the current user's portable account bundle.
func (h *Handler) ExportUser(c echo.Context) error {
	bundle, err := h.service.ExportUser(c.Request().Context(), c.Get("userID").(string))
	if err != nil {
		return writePortabilityError(c, err)
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="progo-account-export.json"`)
	return c.JSON(http.StatusOK, bundle)
}

// ImportProject creates a project from a bounded versioned bundle.
func (h *Handler) ImportProject(c echo.Context) error {
	var bundle ProjectBundle
	if !bindImportBundle(c, &bundle) {
		return nil
	}
	result, err := h.service.ImportProject(c.Request().Context(), c.Get("userID").(string), bundle)
	if err != nil {
		return writePortabilityError(c, err)
	}
	return c.JSON(http.StatusCreated, result)
}

// ImportTickets bulk-imports a bounded bundle into an existing project.
func (h *Handler) ImportTickets(c echo.Context) error {
	projectID, ok := portabilityUUID(c, "projectID")
	if !ok {
		return nil
	}
	var bundle ProjectBundle
	if !bindImportBundle(c, &bundle) {
		return nil
	}
	result, err := h.service.ImportTickets(c.Request().Context(), projectID, c.Get("userID").(string), bundle)
	if err != nil {
		return writePortabilityError(c, err)
	}
	return c.JSON(http.StatusCreated, result)
}

func bindImportBundle(c echo.Context, bundle *ProjectBundle) bool {
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxImportBodyBytes)
	if err := c.Bind(bundle); err != nil {
		_ = c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid or oversized portability bundle", "code": "invalid_import_bundle"})
		return false
	}
	return true
}

func portabilityUUID(c echo.Context, name string) (string, bool) {
	value := c.Param(name)
	if _, err := uuid.Parse(value); err != nil {
		_ = c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid " + name})
		return "", false
	}
	return value, true
}

func writePortabilityError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrInvalidBundle):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error(), "code": "invalid_import_bundle"})
	case errors.Is(err, apperror.ErrForbidden):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrConflict), errors.Is(err, apperror.ErrPrecondition):
		return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "portability operation failed"})
	}
}
