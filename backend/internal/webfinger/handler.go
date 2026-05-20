package webfinger

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

const jrdMediaType = "application/jrd+json"

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(e *echo.Echo) {
	e.GET("/.well-known/webfinger", h.Resolve)
}

func (h *Handler) Resolve(c echo.Context) error {
	jrd, err := h.service.Resolve(c.Request().Context(), c.QueryParam("resource"))
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidResource):
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, ErrNotFound):
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to resolve webfinger resource"})
		}
	}

	raw, err := json.Marshal(jrd)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to encode webfinger response"})
	}
	return c.Blob(http.StatusOK, jrdMediaType, raw)
}
