package ticket

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// Handler exposes ticket HTTP endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates a ticket HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers authenticated ticket routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.POST("/projects/:projectID/tickets", h.Create)
	api.GET("/projects/:projectID/tickets", h.List)
	api.GET("/tickets/:id", h.Get)
	api.PATCH("/tickets/:id", h.Update)
	api.DELETE("/tickets/:id", h.Delete)
	api.GET("/projects/:projectID/graph", h.GetGraph)
	api.POST("/tickets/:id/links", h.AddLink)
	api.DELETE("/links/:linkID", h.RemoveLink)
}

// createTicketRequest is the JSON payload for creating a ticket.
type createTicketRequest struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Priority    string  `json:"priority"`
	Type        string  `json:"type"`
	ParentID    *string `json:"parent_id"`
	AssigneeID  *string `json:"assignee_id"`
}

// Create creates a ticket in a project.
func (h *Handler) Create(c echo.Context) error {
	projectID := c.Param("projectID")

	var req createTicketRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	userID := c.Get("userID").(string)

	serviceReq := CreateTicketRequest{
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		Type:        req.Type,
		ParentID:    req.ParentID,
		AssigneeID:  req.AssigneeID,
	}

	ticket, err := h.service.CreateTicket(c.Request().Context(), serviceReq, projectID, userID)
	if err != nil {
		return writeTicketError(c, err)
	}

	return c.JSON(http.StatusCreated, ticket)
}

// List returns tickets for a project.
func (h *Handler) List(c echo.Context) error {
	projectID := c.Param("projectID")

	userID := c.Get("userID").(string)

	tickets, err := h.service.ListTicketsInProject(c.Request().Context(), projectID, userID)
	if err != nil {
		return writeTicketError(c, err)
	}

	return c.JSON(http.StatusOK, tickets)
}

// updateTicketRequest is the JSON payload for partial ticket updates.
type updateTicketRequest struct {
	Title       *string  `json:"title"`
	Description *string  `json:"description"`
	Status      *string  `json:"status"`
	Priority    *string  `json:"priority"`
	Type        *string  `json:"type"`
	ParentID    **string `json:"parent_id"`
	AssigneeID  **string `json:"assignee_id"`
}

// Get returns a ticket by ID.
func (h *Handler) Get(c echo.Context) error {
	ticketID := c.Param("id")
	userID := c.Get("userID").(string)

	ticket, err := h.service.GetTicketByID(c.Request().Context(), ticketID, userID)
	if err != nil {
		return writeTicketError(c, err)
	}
	return c.JSON(http.StatusOK, ticket)
}

// Update changes ticket fields.
func (h *Handler) Update(c echo.Context) error {
	ticketID := c.Param("id")
	userID := c.Get("userID").(string)

	var req updateTicketRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	serviceReq := UpdateTicketRequest{
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		Priority:    req.Priority,
		Type:        req.Type,
		ParentID:    req.ParentID,
		AssigneeID:  req.AssigneeID,
	}

	if err := h.service.UpdateTicket(c.Request().Context(), serviceReq, ticketID, userID); err != nil {
		return writeTicketError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// Delete removes a ticket.
func (h *Handler) Delete(c echo.Context) error {
	ticketID := c.Param("id")
	userID := c.Get("userID").(string)

	if err := h.service.DeleteTicket(c.Request().Context(), ticketID, userID); err != nil {
		return writeTicketError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// addLinkRequest is the JSON payload for creating a ticket link.
type addLinkRequest struct {
	TargetID string `json:"target_id"`
	LinkType string `json:"link_type"`
}

// AddLink creates a directed link from one ticket to another.
func (h *Handler) AddLink(c echo.Context) error {
	sourceID := c.Param("id")

	var req addLinkRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	userID := c.Get("userID").(string)

	sourceTicket, err := h.service.GetTicketByID(c.Request().Context(), sourceID, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Source ticket not found"})
	}

	err = h.service.AddTicketLink(c.Request().Context(), sourceID, req.TargetID, req.LinkType, sourceTicket.ProjectID, userID)
	if err != nil {
		return writeTicketError(c, err)
	}

	return c.NoContent(http.StatusCreated)
}

// RemoveLink removes a ticket link.
func (h *Handler) RemoveLink(c echo.Context) error {
	linkID := c.Param("linkID")
	userID := c.Get("userID").(string)

	err := h.service.RemoveTicketLink(c.Request().Context(), linkID, "", userID)
	if err != nil {
		return writeTicketError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// GetGraph returns ticket graph data for a project.
func (h *Handler) GetGraph(c echo.Context) error {
	projectID := c.Param("projectID")
	userID := c.Get("userID").(string)

	graph, err := h.service.GetTicketGraph(c.Request().Context(), projectID, userID)
	if err != nil {
		return writeTicketError(c, err)
	}

	return c.JSON(http.StatusOK, graph)
}

// writeTicketError maps ticket service errors to HTTP responses.
func writeTicketError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrInvalidTicketInput):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case strings.Contains(err.Error(), "insufficient permissions"):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	case strings.Contains(err.Error(), "access denied"):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	case strings.Contains(err.Error(), "not found"):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "ticket operation failed"})
	}
}
