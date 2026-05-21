package ticket

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
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
	projectID, ok := uuidParam(c, "projectID", "project id")
	if !ok {
		return nil
	}

	var req createTicketRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := validateCreateTicketIDs(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
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
	projectID, ok := uuidParam(c, "projectID", "project id")
	if !ok {
		return nil
	}

	userID := c.Get("userID").(string)
	options, err := ticketListOptions(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	tickets, err := h.service.ListTicketsInProject(c.Request().Context(), projectID, userID, options)
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
	ticketID, ok := uuidParam(c, "id", "ticket id")
	if !ok {
		return nil
	}
	userID := c.Get("userID").(string)

	ticket, err := h.service.GetTicketByID(c.Request().Context(), ticketID, userID)
	if err != nil {
		return writeTicketError(c, err)
	}
	return c.JSON(http.StatusOK, ticket)
}

// Update changes ticket fields.
func (h *Handler) Update(c echo.Context) error {
	ticketID, ok := uuidParam(c, "id", "ticket id")
	if !ok {
		return nil
	}
	userID := c.Get("userID").(string)

	var req updateTicketRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := validateUpdateTicketIDs(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
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
	ticketID, ok := uuidParam(c, "id", "ticket id")
	if !ok {
		return nil
	}
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
	sourceID, ok := uuidParam(c, "id", "ticket id")
	if !ok {
		return nil
	}

	var req addLinkRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if !validUUID(req.TargetID) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid target id"})
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
	linkID, ok := uuidParam(c, "linkID", "link id")
	if !ok {
		return nil
	}
	userID := c.Get("userID").(string)

	err := h.service.RemoveTicketLink(c.Request().Context(), linkID, "", userID)
	if err != nil {
		return writeTicketError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// GetGraph returns ticket graph data for a project.
func (h *Handler) GetGraph(c echo.Context) error {
	projectID, ok := uuidParam(c, "projectID", "project id")
	if !ok {
		return nil
	}
	userID := c.Get("userID").(string)

	graph, err := h.service.GetTicketGraph(c.Request().Context(), projectID, userID)
	if err != nil {
		return writeTicketError(c, err)
	}

	return c.JSON(http.StatusOK, graph)
}

// ticketListOptions parses bounded ticket list pagination.
func ticketListOptions(c echo.Context) (TicketListOptions, error) {
	limit, err := parseOptionalPositiveInt(c.QueryParam("limit"))
	if err != nil {
		return TicketListOptions{}, ErrInvalidTicketInput
	}
	offset, err := parseOptionalNonNegativeInt(c.QueryParam("offset"))
	if err != nil {
		return TicketListOptions{}, ErrInvalidTicketInput
	}
	return TicketListOptions{Limit: limit, Offset: offset}, nil
}

// validateCreateTicketIDs validates optional UUID references in create requests.
func validateCreateTicketIDs(req createTicketRequest) error {
	if req.ParentID != nil && !validUUID(*req.ParentID) {
		return ErrInvalidTicketInput
	}
	if req.AssigneeID != nil && !validUUID(*req.AssigneeID) {
		return ErrInvalidTicketInput
	}
	return nil
}

// validateUpdateTicketIDs validates optional UUID references in update requests.
func validateUpdateTicketIDs(req updateTicketRequest) error {
	if req.ParentID != nil && *req.ParentID != nil && !validUUID(**req.ParentID) {
		return ErrInvalidTicketInput
	}
	if req.AssigneeID != nil && *req.AssigneeID != nil && !validUUID(**req.AssigneeID) {
		return ErrInvalidTicketInput
	}
	return nil
}

// parseOptionalPositiveInt parses an optional positive pagination value.
func parseOptionalPositiveInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, ErrInvalidTicketInput
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
		return 0, ErrInvalidTicketInput
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
