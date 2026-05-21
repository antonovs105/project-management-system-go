package delivery

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// Handler exposes project-scoped federation delivery inspection endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates a federation delivery HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers authenticated delivery inspection routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/projects/:projectID/deliveries", h.ListProjectDeliveries)
	api.GET("/projects/:projectID/deliveries/summary", h.GetProjectDeliverySummary)
	api.POST("/projects/:projectID/deliveries/:deliveryID/retry", h.RetryProjectDelivery)
}

// ListProjectDeliveries returns delivery attempts for a project.
func (h *Handler) ListProjectDeliveries(c echo.Context) error {
	projectID := c.Param("projectID")
	userID := c.Get("userID").(string)
	options, err := projectDeliveryListOptions(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	deliveries, err := h.service.ListProjectDeliveriesWithOptions(c.Request().Context(), projectID, userID, options)
	if err != nil {
		if errors.Is(err, ErrProjectAccessDenied) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
		}
		if errors.Is(err, ErrInvalidDeliveryFilter) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list project deliveries"})
	}

	return c.JSON(http.StatusOK, deliveries)
}

// GetProjectDeliverySummary returns aggregate delivery state counts for a project.
func (h *Handler) GetProjectDeliverySummary(c echo.Context) error {
	projectID := c.Param("projectID")
	userID := c.Get("userID").(string)

	summary, err := h.service.GetProjectDeliverySummary(c.Request().Context(), projectID, userID)
	if err != nil {
		if errors.Is(err, ErrProjectAccessDenied) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get project delivery summary"})
	}

	return c.JSON(http.StatusOK, summary)
}

// RetryProjectDelivery manually requeues a failed project delivery.
func (h *Handler) RetryProjectDelivery(c echo.Context) error {
	projectID := c.Param("projectID")
	deliveryID := c.Param("deliveryID")
	userID := c.Get("userID").(string)

	delivery, err := h.service.RetryProjectDelivery(c.Request().Context(), projectID, userID, deliveryID)
	if err != nil {
		if errors.Is(err, ErrProjectAccessDenied) || errors.Is(err, ErrDeliveryRetryDenied) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
		}
		if errors.Is(err, ErrDeliveryNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		}
		if errors.Is(err, ErrDeliveryDone) || errors.Is(err, ErrDeliveryRetryUnavailable) {
			return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to retry project delivery"})
	}

	return c.JSON(http.StatusAccepted, delivery)
}

func projectDeliveryListOptions(c echo.Context) (ProjectDeliveryListOptions, error) {
	options := ProjectDeliveryListOptions{
		State: strings.TrimSpace(c.QueryParam("state")),
	}
	if rawLimit := strings.TrimSpace(c.QueryParam("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			return ProjectDeliveryListOptions{}, ErrInvalidDeliveryFilter
		}
		if limit <= 0 {
			return ProjectDeliveryListOptions{}, ErrInvalidDeliveryFilter
		}
		options.Limit = limit
	}
	return NormalizeProjectDeliveryListOptions(options)
}
