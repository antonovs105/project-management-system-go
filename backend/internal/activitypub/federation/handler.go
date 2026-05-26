package federation

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// Handler exposes authenticated personal federation endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates a personal federation HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers authenticated personal federation routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/me/federation/inbox", h.ListInboxActivities)
	api.GET("/me/federation/follows", h.ListRemoteFollows)
}

// ListInboxActivities returns normalized inbox activities for the current user.
func (h *Handler) ListInboxActivities(c echo.Context) error {
	options, err := listOptions(c, false)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	activities, err := h.service.ListInboxActivities(c.Request().Context(), currentUserID(c), options)
	if err != nil {
		return writeFederationError(c, err)
	}
	return c.JSON(http.StatusOK, activities)
}

// ListRemoteFollows returns remote actors followed by the current user.
func (h *Handler) ListRemoteFollows(c echo.Context) error {
	options, err := listOptions(c, true)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	follows, err := h.service.ListRemoteFollows(c.Request().Context(), currentUserID(c), options)
	if err != nil {
		return writeFederationError(c, err)
	}
	return c.JSON(http.StatusOK, follows)
}

// currentUserID extracts the authenticated user identifier from Echo context.
func currentUserID(c echo.Context) string {
	userID, _ := c.Get("userID").(string)
	return userID
}

// listOptions parses pagination and optional follow-state filters.
func listOptions(c echo.Context, allowState bool) (ListOptions, error) {
	options := ListOptions{State: strings.TrimSpace(c.QueryParam("state"))}
	if rawLimit := strings.TrimSpace(c.QueryParam("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 {
			return ListOptions{}, ErrInvalidFilter
		}
		options.Limit = limit
	}
	if rawOffset := strings.TrimSpace(c.QueryParam("offset")); rawOffset != "" {
		offset, err := strconv.Atoi(rawOffset)
		if err != nil || offset < 0 {
			return ListOptions{}, ErrInvalidFilter
		}
		options.Offset = offset
	}
	if !allowState && options.State != "" {
		return ListOptions{}, ErrInvalidFilter
	}
	return options, nil
}

// writeFederationError maps personal federation errors to HTTP responses.
func writeFederationError(c echo.Context, err error) error {
	if errors.Is(err, ErrInvalidFilter) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load federation data"})
}
