package moderation

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	service *Service
}

type blockDomainRequest struct {
	Domain string `json:"domain"`
	Reason string `json:"reason"`
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/admin/federation/domain-blocks", h.ListDomainBlocks)
	api.POST("/admin/federation/domain-blocks", h.BlockDomain)
	api.DELETE("/admin/federation/domain-blocks/:domain", h.UnblockDomain)
	api.GET("/admin/federation/remote-actors", h.ListRemoteActors)
	api.GET("/admin/federation/deliveries", h.ListFederationDeliveries)
	api.POST("/admin/federation/deliveries/:deliveryID/retry", h.RetryFederationDelivery)
}

func (h *Handler) ListDomainBlocks(c echo.Context) error {
	blocks, err := h.service.ListDomainBlocks(c.Request().Context(), currentUserID(c))
	if err != nil {
		return writeModerationError(c, err)
	}
	return c.JSON(http.StatusOK, blocks)
}

func (h *Handler) BlockDomain(c echo.Context) error {
	var req blockDomainRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	block, err := h.service.BlockDomain(c.Request().Context(), currentUserID(c), req.Domain, req.Reason)
	if err != nil {
		return writeModerationError(c, err)
	}
	return c.JSON(http.StatusCreated, block)
}

func (h *Handler) UnblockDomain(c echo.Context) error {
	err := h.service.UnblockDomain(c.Request().Context(), currentUserID(c), c.Param("domain"))
	if err != nil {
		return writeModerationError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) ListRemoteActors(c echo.Context) error {
	options, err := remoteActorListOptions(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	actors, err := h.service.ListRemoteActors(c.Request().Context(), currentUserID(c), options)
	if err != nil {
		return writeModerationError(c, err)
	}
	return c.JSON(http.StatusOK, actors)
}

func (h *Handler) ListFederationDeliveries(c echo.Context) error {
	options, err := federationDeliveryListOptions(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	deliveries, err := h.service.ListFederationDeliveries(c.Request().Context(), currentUserID(c), options)
	if err != nil {
		return writeModerationError(c, err)
	}
	return c.JSON(http.StatusOK, deliveries)
}

func (h *Handler) RetryFederationDelivery(c echo.Context) error {
	retried, err := h.service.RetryFederationDelivery(c.Request().Context(), currentUserID(c), c.Param("deliveryID"))
	if err != nil {
		return writeModerationError(c, err)
	}
	return c.JSON(http.StatusAccepted, retried)
}

func currentUserID(c echo.Context) string {
	userID, _ := c.Get("userID").(string)
	return userID
}

func writeModerationError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrAdminRequired):
		return c.JSON(http.StatusForbidden, map[string]string{"error": ErrAdminRequired.Error()})
	case errors.Is(err, ErrInvalidDomainBlock):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrDomainBlockNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": ErrDomainBlockNotFound.Error()})
	case errors.Is(err, ErrInvalidFilter):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, delivery.ErrDeliveryNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": delivery.ErrDeliveryNotFound.Error()})
	case errors.Is(err, delivery.ErrDeliveryDone), errors.Is(err, delivery.ErrDeliveryRetryUnavailable):
		return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "federation moderation operation failed"})
	}
}

func remoteActorListOptions(c echo.Context) (RemoteActorListOptions, error) {
	fetchErrorOnly, err := parseOptionalBool(c.QueryParam("fetch_error"))
	if err != nil {
		return RemoteActorListOptions{}, ErrInvalidFilter
	}
	limit, err := parseOptionalLimit(c.QueryParam("limit"))
	if err != nil {
		return RemoteActorListOptions{}, ErrInvalidFilter
	}
	return RemoteActorListOptions{FetchErrorOnly: fetchErrorOnly, Limit: limit}, nil
}

func federationDeliveryListOptions(c echo.Context) (FederationDeliveryListOptions, error) {
	limit, err := parseOptionalLimit(c.QueryParam("limit"))
	if err != nil {
		return FederationDeliveryListOptions{}, ErrInvalidFilter
	}
	return FederationDeliveryListOptions{
		State:       strings.TrimSpace(c.QueryParam("state")),
		FailureKind: strings.TrimSpace(c.QueryParam("failure_kind")),
		Limit:       limit,
	}, nil
}

func parseOptionalBool(raw string) (bool, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return false, nil
	}
	switch raw {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, ErrInvalidFilter
	}
}

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
