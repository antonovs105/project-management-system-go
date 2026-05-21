package remoteinbox

import (
	"errors"
	"io"
	"net/http"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/labstack/echo/v4"
)

// Handler exposes remote ActivityPub inbox POST endpoints.
type Handler struct {
	service *Service
	cfg     activitypub.Config
}

// NewHandler creates a remote inbox HTTP handler.
func NewHandler(service *Service, cfg activitypub.Config) *Handler {
	return &Handler{service: service, cfg: cfg}
}

// RegisterRoutes registers user and project inbox POST routes.
func (h *Handler) RegisterRoutes(e *echo.Echo, middleware ...echo.MiddlewareFunc) {
	e.POST("/users/:username/inbox", h.ReceiveUserInbox, middleware...)
	e.POST("/projects/:id/inbox", h.ReceiveProjectInbox, middleware...)
}

// ReceiveUserInbox accepts an inbound activity addressed to a local user actor.
func (h *Handler) ReceiveUserInbox(c echo.Context) error {
	return h.receive(c, activitypub.UserAPID(h.cfg, c.Param("username")))
}

// ReceiveProjectInbox accepts an inbound activity addressed to a local project actor.
func (h *Handler) ReceiveProjectInbox(c echo.Context) error {
	return h.receive(c, activitypub.ProjectAPID(h.cfg, c.Param("id")))
}

// receive validates request metadata, reads the bounded body, and delegates inbox handling.
func (h *Handler) receive(c echo.Context, targetAPID string) error {
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	if !isActivityMediaType(c.Request().Header.Get(echo.HeaderContentType)) {
		return c.JSON(http.StatusUnsupportedMediaType, errorResponse(ErrUnsupportedMedia))
	}

	body, err := readLimitedBody(c.Request().Body, h.service.MaxBodyBytes())
	if err != nil {
		if errors.Is(err, ErrBodyTooLarge) {
			return c.JSON(http.StatusRequestEntityTooLarge, errorResponse(ErrBodyTooLarge))
		}
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to read inbox activity"})
	}

	accepted, err := h.service.Receive(c.Request().Context(), c.Request(), targetAPID, body)
	if err != nil {
		switch {
		case errors.Is(err, ErrTargetNotFound):
			return c.JSON(http.StatusNotFound, errorResponse(ErrTargetNotFound))
		case errors.Is(err, ErrUnauthorized):
			return c.JSON(http.StatusUnauthorized, errorResponse(ErrUnauthorized))
		case errors.Is(err, ErrForbiddenActor):
			return c.JSON(http.StatusForbidden, errorResponse(ErrForbiddenActor))
		case errors.Is(err, ErrBlockedDomain):
			return c.JSON(http.StatusForbidden, errorResponse(ErrBlockedDomain))
		case errors.Is(err, ErrInvalidActivity):
			return c.JSON(http.StatusBadRequest, errorResponse(ErrInvalidActivity))
		case errors.Is(err, ErrUnsupportedActivity):
			return c.JSON(http.StatusUnprocessableEntity, errorResponse(ErrUnsupportedActivity))
		case errors.Is(err, ErrActivityConflict):
			return c.JSON(http.StatusConflict, errorResponse(ErrActivityConflict))
		default:
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to receive inbox activity"})
		}
	}

	return c.JSON(http.StatusAccepted, accepted)
}

// errorResponse converts a service error into the common JSON error envelope.
func errorResponse(err error) map[string]string {
	return map[string]string{"error": err.Error()}
}

// readLimitedBody reads an inbox body while enforcing the configured size limit.
func readLimitedBody(body io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(body, maxBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, ErrBodyTooLarge
	}
	return raw, nil
}
