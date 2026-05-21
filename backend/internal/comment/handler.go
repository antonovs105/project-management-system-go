package comment

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// Handler exposes comment HTTP endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates a comment HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers authenticated comment routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.POST("/tickets/:id/comments", h.Create)
	api.GET("/tickets/:id/comments", h.List)
	api.DELETE("/comments/:id", h.Delete)
}

// createCommentRequest is the JSON payload for adding a comment to a ticket.
type createCommentRequest struct {
	Content string `json:"content"`
}

// Create adds a comment to a ticket.
func (h *Handler) Create(c echo.Context) error {
	ticketID := c.Param("id")
	userID := c.Get("userID").(string)

	var req createCommentRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	comment, err := h.service.CreateComment(c.Request().Context(), ticketID, userID, req.Content)
	if err != nil {
		return writeCommentError(c, err)
	}

	return c.JSON(http.StatusCreated, comment)
}

// List returns comments for a ticket.
func (h *Handler) List(c echo.Context) error {
	ticketID := c.Param("id")
	userID := c.Get("userID").(string)

	comments, err := h.service.ListComments(c.Request().Context(), ticketID, userID)
	if err != nil {
		return writeCommentError(c, err)
	}

	return c.JSON(http.StatusOK, comments)
}

// Delete removes a comment by ID.
func (h *Handler) Delete(c echo.Context) error {
	commentID := c.Param("id")
	userID := c.Get("userID").(string)

	if err := h.service.DeleteComment(c.Request().Context(), commentID, userID); err != nil {
		return writeCommentError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// writeCommentError maps comment service errors to HTTP responses.
func writeCommentError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrInvalidCommentInput):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case strings.Contains(err.Error(), "insufficient permissions"):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	case strings.Contains(err.Error(), "not found"), strings.Contains(err.Error(), "access denied"):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "comment operation failed"})
	}
}
