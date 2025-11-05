package ticket

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type createTicketRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	AssigneeID  *int64 `json:"assignee_id"`
}

// Create handler for POST /api/projects/:projectID/tickets
func (h *Handler) Create(c echo.Context) error {
	projectID, err := strconv.ParseInt(c.Param("projectID"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid project ID"})
	}

	var req createTicketRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	userID := c.Get("userID").(int64)

	serviceReq := CreateTicketRequest{
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		AssigneeID:  req.AssigneeID,
	}

	ticket, err := h.service.CreateTicket(c.Request().Context(), serviceReq, projectID, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, ticket)
}

// List handler for GET /api/projects/:projectID/tickets
func (h *Handler) List(c echo.Context) error {
	projectID, err := strconv.ParseInt(c.Param("projectID"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid project ID"})
	}

	userID := c.Get("userID").(int64)

	tickets, err := h.service.ListTicketsInProject(c.Request().Context(), projectID, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, tickets)
}
