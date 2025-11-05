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

type updateTicketRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	Priority    *string `json:"priority"`
	AssigneeID  **int64 `json:"assignee_id"`
}

// Get handler for GET /api/tickets/:id
func (h *Handler) Get(c echo.Context) error {
	ticketID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid ticket ID"})
	}
	userID := c.Get("userID").(int64)

	ticket, err := h.service.GetTicketByID(c.Request().Context(), ticketID, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, ticket)
}

// Update handler for PATCH /api/tickets/:id
func (h *Handler) Update(c echo.Context) error {
	ticketID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid ticket ID"})
	}
	userID := c.Get("userID").(int64)

	var req updateTicketRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	serviceReq := UpdateTicketRequest{
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		Priority:    req.Priority,
		AssigneeID:  req.AssigneeID,
	}

	err = h.service.UpdateTicket(c.Request().Context(), serviceReq, ticketID, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// Delete handler for DELETE /api/tickets/:id
func (h *Handler) Delete(c echo.Context) error {
	ticketID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid ticket ID"})
	}
	userID := c.Get("userID").(int64)

	err = h.service.DeleteTicket(c.Request().Context(), ticketID, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}
