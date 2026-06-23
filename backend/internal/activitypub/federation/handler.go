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
	api.POST("/me/federation/discover", h.DiscoverRemoteActor)
	api.POST("/me/federation/follows", h.FollowRemoteActor)
	api.GET("/me/remote-project-invites", h.ListRemoteProjectInvites)
	api.POST("/me/remote-project-invites/:id/accept", h.AcceptRemoteProjectInvite)
	api.POST("/me/remote-project-invites/:id/reject", h.RejectRemoteProjectInvite)
}

// remoteActorRequest accepts remote actor identifiers for discovery and follow actions.
type remoteActorRequest struct {
	Resource string `json:"resource"`
	Target   string `json:"target"`
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

// ListRemoteProjectInvites returns remote project invites addressed to the current user.
func (h *Handler) ListRemoteProjectInvites(c echo.Context) error {
	options, err := listOptions(c, true)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	invites, err := h.service.ListRemoteProjectInvites(c.Request().Context(), currentUserID(c), options)
	if err != nil {
		return writeFederationError(c, err)
	}
	return c.JSON(http.StatusOK, invites)
}

// AcceptRemoteProjectInvite accepts a pending remote project invite for the current user.
func (h *Handler) AcceptRemoteProjectInvite(c echo.Context) error {
	result, err := h.service.AcceptRemoteProjectInvite(c.Request().Context(), currentUserID(c), strings.TrimSpace(c.Param("id")))
	if err != nil {
		return writeFederationError(c, err)
	}
	return c.JSON(http.StatusAccepted, result)
}

// RejectRemoteProjectInvite rejects a pending remote project invite for the current user.
func (h *Handler) RejectRemoteProjectInvite(c echo.Context) error {
	result, err := h.service.RejectRemoteProjectInvite(c.Request().Context(), currentUserID(c), strings.TrimSpace(c.Param("id")))
	if err != nil {
		return writeFederationError(c, err)
	}
	return c.JSON(http.StatusAccepted, result)
}

// DiscoverRemoteActor resolves and caches a remote actor for the current user.
func (h *Handler) DiscoverRemoteActor(c echo.Context) error {
	var req remoteActorRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid federation request"})
	}
	actor, err := h.service.DiscoverRemoteActor(c.Request().Context(), req.resource())
	if err != nil {
		return writeFederationError(c, err)
	}
	return c.JSON(http.StatusOK, actor)
}

// FollowRemoteActor creates and queues a Follow from the current user actor.
func (h *Handler) FollowRemoteActor(c echo.Context) error {
	var req remoteActorRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid federation request"})
	}
	result, err := h.service.FollowRemoteActor(c.Request().Context(), currentUserID(c), req.resource())
	if err != nil {
		return writeFederationError(c, err)
	}
	if result.Created {
		return c.JSON(http.StatusAccepted, result)
	}
	return c.JSON(http.StatusOK, result)
}

// resource returns either accepted request field for discovery/follow targets.
func (r remoteActorRequest) resource() string {
	if strings.TrimSpace(r.Resource) != "" {
		return r.Resource
	}
	return r.Target
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
	switch {
	case errors.Is(err, ErrInvalidFilter), errors.Is(err, ErrInvalidRemoteResource):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrRemoteActorUnavailable):
		return c.JSON(http.StatusBadGateway, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrLocalActorNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrRemoteInviteNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrRemoteInviteNotPending):
		return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load federation data"})
	}
}
