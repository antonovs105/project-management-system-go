package comment

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/apiresponse"
	"github.com/antonovs105/project-management-system-go/internal/apperror"
	"github.com/google/uuid"
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
	ticketID, ok := uuidParam(c, "id", "ticket id")
	if !ok {
		return nil
	}
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
	ticketID, ok := uuidParam(c, "id", "ticket id")
	if !ok {
		return nil
	}
	userID := c.Get("userID").(string)
	options, err := commentListOptions(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	comments, err := h.service.ListComments(c.Request().Context(), ticketID, userID, options)
	if err != nil {
		return writeCommentError(c, err)
	}

	return apiresponse.WriteOffsetPage(c, http.StatusOK, comments, normalizeCommentListLimit(options.Limit), normalizeCommentListOffset(options.Offset))
}

// Delete removes a comment by ID.
func (h *Handler) Delete(c echo.Context) error {
	commentID, ok := uuidParam(c, "id", "comment id")
	if !ok {
		return nil
	}
	userID := c.Get("userID").(string)

	if err := h.service.DeleteComment(c.Request().Context(), commentID, userID); err != nil {
		return writeCommentError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// commentListOptions parses bounded comment list pagination.
func commentListOptions(c echo.Context) (CommentListOptions, error) {
	limit, err := parseOptionalPositiveInt(c.QueryParam("limit"))
	if err != nil {
		return CommentListOptions{}, ErrInvalidCommentInput
	}
	offset, err := parseOptionalNonNegativeInt(c.QueryParam("offset"))
	if err != nil {
		return CommentListOptions{}, ErrInvalidCommentInput
	}
	return CommentListOptions{Limit: limit, Offset: offset}, nil
}

// parseOptionalPositiveInt parses an optional positive pagination value.
func parseOptionalPositiveInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, ErrInvalidCommentInput
	}
	return value, nil
}

// parseOptionalNonNegativeInt parses an optional non-negative pagination value.
func parseOptionalNonNegativeInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, ErrInvalidCommentInput
	}
	return value, nil
}

// uuidParam extracts and validates a UUID path parameter.
func uuidParam(c echo.Context, name string, label string) (string, bool) {
	value := c.Param(name)
	if !validUUID(value) {
		_ = c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid " + label})
		return "", false
	}
	return value, true
}

// validUUID reports whether value is a valid UUID.
func validUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

// writeCommentError maps comment service errors to HTTP responses.
func writeCommentError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrInvalidCommentInput):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrForbidden):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "comment operation failed"})
	}
}
