package comment

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type createCommentRequest struct {
	Content string `json:"content"`
}

func (h *Handler) Create(c echo.Context) error {
	ticketID := c.Param("id")
	userID := c.Get("userID").(string)

	var req createCommentRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	comment, err := h.service.CreateComment(c.Request().Context(), ticketID, userID, req.Content)
	if err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, comment)
}

func (h *Handler) List(c echo.Context) error {
	ticketID := c.Param("id")
	userID := c.Get("userID").(string)

	comments, err := h.service.ListComments(c.Request().Context(), ticketID, userID)
	if err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, comments)
}
