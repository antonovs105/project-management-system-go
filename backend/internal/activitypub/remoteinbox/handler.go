package remoteinbox

import (
	"errors"
	"io"
	"net/http"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	service *Service
	cfg     activitypub.Config
}

func NewHandler(service *Service, cfg activitypub.Config) *Handler {
	return &Handler{service: service, cfg: cfg}
}

func (h *Handler) RegisterRoutes(e *echo.Echo) {
	e.POST("/users/:username/inbox", h.ReceiveUserInbox)
	e.POST("/projects/:id/inbox", h.ReceiveProjectInbox)
}

func (h *Handler) ReceiveUserInbox(c echo.Context) error {
	return h.receive(c, activitypub.UserAPID(h.cfg, c.Param("username")))
}

func (h *Handler) ReceiveProjectInbox(c echo.Context) error {
	return h.receive(c, activitypub.ProjectAPID(h.cfg, c.Param("id")))
}

func (h *Handler) receive(c echo.Context, targetAPID string) error {
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

func errorResponse(err error) map[string]string {
	return map[string]string{"error": err.Error()}
}

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
