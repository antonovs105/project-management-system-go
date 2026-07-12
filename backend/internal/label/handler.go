package label

import (
	"errors"
	"net/http"

	"github.com/antonovs105/project-management-system-go/internal/apperror"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Handler exposes project label endpoints.
type Handler struct{ service *Service }

// NewHandler creates a label handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// RegisterRoutes mounts authenticated label routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/projects/:projectID/labels", h.List)
	api.POST("/projects/:projectID/labels", h.Create)
	api.DELETE("/projects/:projectID/labels/:labelID", h.Delete)
}

// List returns project labels.
func (h *Handler) List(c echo.Context) error {
	projectID, ok := labelUUID(c, "projectID")
	if !ok {
		return nil
	}
	items, err := h.service.List(c.Request().Context(), projectID, c.Get("userID").(string))
	if err != nil {
		return writeLabelError(c, err)
	}
	return c.JSON(http.StatusOK, items)
}

// Create adds a project label.
func (h *Handler) Create(c echo.Context) error {
	projectID, ok := labelUUID(c, "projectID")
	if !ok {
		return nil
	}
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	item, err := h.service.Create(c.Request().Context(), projectID, c.Get("userID").(string), req.Name, req.Color)
	if err != nil {
		return writeLabelError(c, err)
	}
	return c.JSON(http.StatusCreated, item)
}

// Delete removes a project label.
func (h *Handler) Delete(c echo.Context) error {
	projectID, ok := labelUUID(c, "projectID")
	if !ok {
		return nil
	}
	labelID, ok := labelUUID(c, "labelID")
	if !ok {
		return nil
	}
	if err := h.service.Delete(c.Request().Context(), projectID, labelID, c.Get("userID").(string)); err != nil {
		return writeLabelError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// labelUUID validates a UUID route parameter.
func labelUUID(c echo.Context, name string) (string, bool) {
	value := c.Param(name)
	if _, err := uuid.Parse(value); err != nil {
		_ = c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid " + name})
		return "", false
	}
	return value, true
}

// writeLabelError maps typed label errors to HTTP status codes.
func writeLabelError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrForbidden):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrConflict):
		return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "label operation failed"})
	}
}
