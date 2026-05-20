package delivery

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/projects/:projectID/deliveries", h.ListProjectDeliveries)
	api.POST("/projects/:projectID/deliveries/:deliveryID/retry", h.RetryProjectDelivery)
}

func (h *Handler) ListProjectDeliveries(c echo.Context) error {
	projectID := c.Param("projectID")
	userID := c.Get("userID").(string)

	deliveries, err := h.service.ListProjectDeliveries(c.Request().Context(), projectID, userID)
	if err != nil {
		if errors.Is(err, ErrProjectAccessDenied) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list project deliveries"})
	}

	return c.JSON(http.StatusOK, deliveries)
}

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
