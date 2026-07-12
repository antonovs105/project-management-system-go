package notification

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Handler exposes authenticated notification endpoints.
type Handler struct {
	service *Service
	events  EventSubscriber
}

// HandlerOption customizes the handler.
type HandlerOption func(*Handler)

// WithEventSubscriber attaches realtime notification streams.
func WithEventSubscriber(events EventSubscriber) HandlerOption {
	return func(h *Handler) {
		h.events = events
	}
}

// NewHandler creates a notification handler.
func NewHandler(service *Service, options ...HandlerOption) *Handler {
	handler := &Handler{service: service}
	for _, option := range options {
		option(handler)
	}
	return handler
}

// RegisterRoutes registers authenticated notification routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/me/notifications", h.List)
	api.GET("/me/notifications/events", h.StreamEvents)
	api.PATCH("/me/notifications/:id/read", h.MarkRead)
	api.POST("/me/notifications/read-all", h.MarkAllRead)
	api.GET("/me/notification-preferences", h.ListPreferences)
	api.PUT("/me/notification-preferences/:type", h.UpdatePreference)
}

// ListPreferences returns the complete notification delivery catalog.
func (h *Handler) ListPreferences(c echo.Context) error {
	values, err := h.service.ListPreferences(c.Request().Context(), currentUserID(c))
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(http.StatusOK, values)
}

// UpdatePreference stores one in-app/email preference override.
func (h *Handler) UpdatePreference(c echo.Context) error {
	var request struct {
		InAppEnabled bool `json:"in_app_enabled"`
		EmailEnabled bool `json:"email_enabled"`
	}
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": ErrInvalidInput.Error()})
	}
	value, err := h.service.UpdatePreference(c.Request().Context(), currentUserID(c), Preference{
		Type:         strings.TrimSpace(c.Param("type")),
		InAppEnabled: request.InAppEnabled,
		EmailEnabled: request.EmailEnabled,
	})
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(http.StatusOK, value)
}

// List returns recent notifications for the current user.
func (h *Handler) List(c echo.Context) error {
	options, err := parseListOptions(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	notifications, err := h.service.List(c.Request().Context(), currentUserID(c), options)
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(http.StatusOK, notifications)
}

// StreamEvents streams notification events for the current user.
func (h *Handler) StreamEvents(c echo.Context) error {
	if h.events == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "notification event stream is not configured"})
	}
	userID := currentUserID(c)
	response := c.Response()
	flusher, ok := response.Writer.(http.Flusher)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "streaming is not supported"})
	}
	response.Header().Set(echo.HeaderContentType, "text/event-stream")
	response.Header().Set(echo.HeaderCacheControl, "no-cache")
	response.Header().Set("Connection", "keep-alive")
	response.WriteHeader(http.StatusOK)

	notifications, unsubscribe := h.events.SubscribeNotifications(userID)
	defer unsubscribe()

	if err := writeSSE(response.Writer, "ready", "", map[string]string{"user_id": userID}); err != nil {
		return err
	}
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case notification := <-notifications:
			if err := writeSSE(response.Writer, "notification.created", notification.ID, notification); err != nil {
				return err
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := io.WriteString(response.Writer, ": ping\n\n"); err != nil {
				return err
			}
			flusher.Flush()
		}
	}
}

// MarkRead marks one notification as read.
func (h *Handler) MarkRead(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": ErrInvalidInput.Error()})
	}
	notification, err := h.service.MarkRead(c.Request().Context(), currentUserID(c), id)
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(http.StatusOK, notification)
}

// MarkAllRead marks all notifications as read.
func (h *Handler) MarkAllRead(c echo.Context) error {
	if err := h.service.MarkAllRead(c.Request().Context(), currentUserID(c)); err != nil {
		return writeError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// parseListOptions parses bounded notification list filters.
func parseListOptions(c echo.Context) (ListOptions, error) {
	limit, err := parseOptionalNonNegativeInt(c.QueryParam("limit"))
	if err != nil {
		return ListOptions{}, ErrInvalidInput
	}
	offset, err := parseOptionalNonNegativeInt(c.QueryParam("offset"))
	if err != nil {
		return ListOptions{}, ErrInvalidInput
	}
	unreadOnly := false
	rawUnread := strings.TrimSpace(c.QueryParam("unread"))
	if rawUnread != "" {
		value, err := strconv.ParseBool(rawUnread)
		if err != nil {
			return ListOptions{}, ErrInvalidInput
		}
		unreadOnly = value
	}
	return ListOptions{Limit: limit, Offset: offset, UnreadOnly: unreadOnly}, nil
}

// parseOptionalNonNegativeInt parses an optional non-negative integer.
func parseOptionalNonNegativeInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, ErrInvalidInput
	}
	return value, nil
}

// writeSSE writes one server-sent event frame.
func writeSSE(w io.Writer, eventName, eventID string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if eventID != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", eventID); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, raw)
	return err
}

// currentUserID returns the authenticated user ID stored by JWT middleware.
func currentUserID(c echo.Context) string {
	userID, _ := c.Get("userID").(string)
	return userID
}

// writeError maps notification service errors to HTTP responses.
func writeError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "notification operation failed"})
	}
}
