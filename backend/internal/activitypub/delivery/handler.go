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
