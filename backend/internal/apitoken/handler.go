package apitoken

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

// Handler exposes authenticated token lifecycle endpoints.
type Handler struct{ service *Service }

// NewHandler creates an API token handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// RegisterRoutes mounts current-user API token routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/me/api-tokens", h.List)
	api.POST("/me/api-tokens", h.Create)
	api.DELETE("/me/api-tokens/:tokenID", h.Revoke)
}

// List returns token metadata without plaintext secrets.
func (h *Handler) List(c echo.Context) error {
	values, err := h.service.List(c.Request().Context(), c.Get("userID").(string))
	if err != nil {
		return writeTokenError(c, err)
	}
	return c.JSON(http.StatusOK, values)
}

// Create returns a new token secret exactly once.
func (h *Handler) Create(c echo.Context) error {
	var request CreateRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	created, err := h.service.Create(c.Request().Context(), c.Get("userID").(string), request)
	if err != nil {
		return writeTokenError(c, err)
	}
	return c.JSON(http.StatusCreated, created)
}

// Revoke invalidates an owned token.
func (h *Handler) Revoke(c echo.Context) error {
	if err := h.service.Revoke(c.Request().Context(), c.Get("userID").(string), c.Param("tokenID")); err != nil {
		return writeTokenError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func writeTokenError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": ErrNotFound.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "api token operation failed"})
	}
}
