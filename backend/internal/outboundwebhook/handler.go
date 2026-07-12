package outboundwebhook

import (
	"errors"
	"net/http"

	"github.com/antonovs105/project-management-system-go/internal/apperror"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Handler exposes project webhook configuration and diagnostics.
type Handler struct{ service *Service }

// NewHandler creates an outbound webhook handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// RegisterRoutes mounts project webhook routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/projects/:projectID/webhooks", h.List)
	api.POST("/projects/:projectID/webhooks", h.Create)
	api.DELETE("/projects/:projectID/webhooks/:webhookID", h.Delete)
	api.GET("/projects/:projectID/webhook-deliveries", h.ListDeliveries)
	api.POST("/projects/:projectID/webhook-deliveries/:deliveryID/retry", h.Retry)
}

// List returns project webhook metadata.
func (h *Handler) List(c echo.Context) error {
	projectID, ok := webhookUUID(c, "projectID")
	if !ok {
		return nil
	}
	values, err := h.service.List(c.Request().Context(), projectID, c.Get("userID").(string))
	if err != nil {
		return writeWebhookError(c, err)
	}
	return c.JSON(http.StatusOK, values)
}

// Create stores a callback and returns its signing secret once.
func (h *Handler) Create(c echo.Context) error {
	projectID, ok := webhookUUID(c, "projectID")
	if !ok {
		return nil
	}
	var request struct {
		Name      string   `json:"name"`
		TargetURL string   `json:"target_url"`
		Events    []string `json:"events"`
	}
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	created, err := h.service.Create(c.Request().Context(), projectID, c.Get("userID").(string), request.Name, request.TargetURL, request.Events)
	if err != nil {
		return writeWebhookError(c, err)
	}
	return c.JSON(http.StatusCreated, created)
}

// Delete removes a callback.
func (h *Handler) Delete(c echo.Context) error {
	projectID, ok := webhookUUID(c, "projectID")
	if !ok {
		return nil
	}
	if err := h.service.Delete(c.Request().Context(), projectID, c.Param("webhookID"), c.Get("userID").(string)); err != nil {
		return writeWebhookError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// ListDeliveries returns recent callback delivery diagnostics.
func (h *Handler) ListDeliveries(c echo.Context) error {
	projectID, ok := webhookUUID(c, "projectID")
	if !ok {
		return nil
	}
	values, err := h.service.ListDeliveries(c.Request().Context(), projectID, c.Get("userID").(string))
	if err != nil {
		return writeWebhookError(c, err)
	}
	return c.JSON(http.StatusOK, values)
}

// Retry reschedules a failed callback delivery.
func (h *Handler) Retry(c echo.Context) error {
	projectID, ok := webhookUUID(c, "projectID")
	if !ok {
		return nil
	}
	if err := h.service.Retry(c.Request().Context(), projectID, c.Param("deliveryID"), c.Get("userID").(string)); err != nil {
		return writeWebhookError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func webhookUUID(c echo.Context, name string) (string, bool) {
	value := c.Param(name)
	if _, err := uuid.Parse(value); err != nil {
		_ = c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid " + name})
		return "", false
	}
	return value, true
}

func writeWebhookError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrForbidden):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrNotFound), errors.Is(err, ErrNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "outbound webhook operation failed"})
	}
}
