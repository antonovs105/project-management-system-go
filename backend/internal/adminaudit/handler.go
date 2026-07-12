package adminaudit

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/apiresponse"
	"github.com/labstack/echo/v4"
)

// Handler exposes administrative audit HTTP endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates an audit HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers audit routes on an authenticated API group.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/admin/audit-events", h.ListEvents)
}

// ListEvents returns audit events visible to the current admin.
func (h *Handler) ListEvents(c echo.Context) error {
	options, err := listOptions(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	events, err := h.service.ListEvents(c.Request().Context(), currentUserID(c), options)
	if err != nil {
		return writeAuditError(c, err)
	}
	return apiresponse.WriteOffsetPage(c, http.StatusOK, events, options.Limit, options.Offset)
}

// currentUserID extracts the authenticated user identifier from Echo context.
func currentUserID(c echo.Context) string {
	userID, _ := c.Get("userID").(string)
	return userID
}

// listOptions parses audit-event query filters.
func listOptions(c echo.Context) (ListOptions, error) {
	limit, err := parseOptionalLimit(c.QueryParam("limit"))
	if err != nil {
		return ListOptions{}, ErrInvalidFilter
	}
	offset, err := parseOptionalOffset(c.QueryParam("offset"))
	if err != nil {
		return ListOptions{}, ErrInvalidFilter
	}
	return ListOptions{
		Action:      strings.TrimSpace(c.QueryParam("action")),
		ActorUserID: strings.TrimSpace(c.QueryParam("actor_user_id")),
		TargetType:  strings.TrimSpace(c.QueryParam("target_type")),
		Limit:       limit,
		Offset:      offset,
	}, nil
}

// parseOptionalLimit parses a positive optional list limit.
func parseOptionalLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, ErrInvalidFilter
	}
	return limit, nil
}

// parseOptionalOffset parses a non-negative optional list offset.
func parseOptionalOffset(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(raw)
	if err != nil || offset < 0 {
		return 0, ErrInvalidFilter
	}
	return offset, nil
}

// writeAuditError maps audit service errors to HTTP responses.
func writeAuditError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrAdminRequired):
		return c.JSON(http.StatusForbidden, map[string]string{"error": ErrAdminRequired.Error()})
	case errors.Is(err, ErrInvalidFilter):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "admin audit operation failed"})
	}
}
