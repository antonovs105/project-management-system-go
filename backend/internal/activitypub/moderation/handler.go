package moderation

import (
	"errors"
	"net/http"

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
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "federation moderation operation failed"})
	}
}
