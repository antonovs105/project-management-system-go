package ticket

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/apiresponse"
	"github.com/antonovs105/project-management-system-go/internal/apperror"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Handler exposes ticket HTTP endpoints.
type Handler struct {
	service *Service
	events  EventSubscriber
}

// HandlerOption customizes the ticket HTTP handler.
type HandlerOption func(*Handler)

// WithEventSubscriber attaches the realtime event subscriber.
func WithEventSubscriber(events EventSubscriber) HandlerOption {
	return func(h *Handler) {
		h.events = events
	}
}

// NewHandler creates a ticket HTTP handler.
func NewHandler(service *Service, options ...HandlerOption) *Handler {
	handler := &Handler{service: service}
	for _, option := range options {
		option(handler)
	}
	return handler
}

// RegisterRoutes registers authenticated ticket routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.POST("/projects/:projectID/tickets", h.Create)
	api.GET("/projects/:projectID/tickets", h.List)
	api.GET("/projects/:projectID/tickets/events", h.StreamEvents)
	api.GET("/tickets/:id", h.Get)
	api.PATCH("/tickets/:id", h.Update)
	api.POST("/tickets/:id/move", h.Move)
	api.DELETE("/tickets/:id", h.Delete)
	api.GET("/projects/:projectID/graph", h.GetGraph)
	api.POST("/tickets/:id/links", h.AddLink)
	api.DELETE("/links/:linkID", h.RemoveLink)
}

// createTicketRequest is the JSON payload for creating a ticket.
type createTicketRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    string   `json:"priority"`
	Type        string   `json:"type"`
	ParentID    *string  `json:"parent_id"`
	AssigneeID  *string  `json:"assignee_id"`
	DueDate     string   `json:"due_date"`
	LabelIDs    []string `json:"label_ids"`
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
		DueDate:     req.DueDate,
		LabelIDs:    req.LabelIDs,
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

// StreamEvents streams project ticket changes as server-sent events.
func (h *Handler) StreamEvents(c echo.Context) error {
	if h.events == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "ticket event stream is not configured"})
	}
	projectID, ok := uuidParam(c, "projectID", "project id")
	if !ok {
		return nil
	}
	userID := c.Get("userID").(string)
	if _, err := h.service.projectService.GetProjectByID(c.Request().Context(), projectID, userID); err != nil {
		return writeTicketError(c, err)
	}

	response := c.Response()
	flusher, ok := response.Writer.(http.Flusher)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "streaming is not supported"})
	}
	response.Header().Set(echo.HeaderContentType, "text/event-stream")
	response.Header().Set(echo.HeaderCacheControl, "no-cache")
	response.Header().Set("Connection", "keep-alive")
	response.WriteHeader(http.StatusOK)

	events, unsubscribe := h.events.SubscribeTicketEvents(projectID)
	defer unsubscribe()

	if err := writeSSE(response.Writer, "ready", "", map[string]string{"project_id": projectID}); err != nil {
		return err
	}
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case event := <-events:
			if err := writeSSE(response.Writer, event.Type, event.ID, event); err != nil {
				return err
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := io.WriteString(response.Writer, ": ping\n\n"); err != nil {
				return err
			}
			flusher.Flush()
		}
	}
}

// updateTicketRequest is the JSON payload for partial ticket updates.
type updateTicketRequest struct {
	Title       *string   `json:"title"`
	Description *string   `json:"description"`
	Status      *string   `json:"status"`
	Priority    *string   `json:"priority"`
	Type        *string   `json:"type"`
	ParentID    **string  `json:"parent_id"`
	AssigneeID  **string  `json:"assignee_id"`
	DueDate     *string   `json:"due_date"`
	LabelIDs    *[]string `json:"label_ids"`
}

// moveTicketRequest is the JSON payload for moving a ticket on the board.
type moveTicketRequest struct {
	Status         string  `json:"status"`
	BeforeTicketID *string `json:"before_ticket_id"`
	AfterTicketID  *string `json:"after_ticket_id"`
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
	apiresponse.SetVersionETag(c, ticket.Version)
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
	version, err := apiresponse.ExpectedVersion(c)
	if err != nil {
		return c.JSON(http.StatusPreconditionRequired, map[string]string{"error": err.Error(), "code": "if_match_required"})
	}
	if err := validateUpdateTicketIDs(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	serviceReq := UpdateTicketRequest{
		Title:           req.Title,
		Description:     req.Description,
		Status:          req.Status,
		Priority:        req.Priority,
		Type:            req.Type,
		ParentID:        req.ParentID,
		AssigneeID:      req.AssigneeID,
		DueDate:         req.DueDate,
		LabelIDs:        req.LabelIDs,
		ExpectedVersion: version,
	}

	if err := h.service.UpdateTicket(c.Request().Context(), serviceReq, ticketID, userID); err != nil {
		return writeTicketError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// Move reorders a ticket within or across board status groups.
func (h *Handler) Move(c echo.Context) error {
	ticketID, ok := uuidParam(c, "id", "ticket id")
	if !ok {
		return nil
	}
	userID := c.Get("userID").(string)

	var req moveTicketRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := validateMoveTicketIDs(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	version, err := apiresponse.ExpectedVersion(c)
	if err != nil {
		return c.JSON(http.StatusPreconditionRequired, map[string]string{"error": err.Error(), "code": "if_match_required"})
	}

	moved, err := h.service.MoveTicket(c.Request().Context(), MoveTicketRequest{
		Status:          req.Status,
		BeforeTicketID:  req.BeforeTicketID,
		AfterTicketID:   req.AfterTicketID,
		ExpectedVersion: version,
	}, ticketID, userID)
	if err != nil {
		return writeTicketError(c, err)
	}
	return c.JSON(http.StatusOK, moved)
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

	options, err := graphOptions(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	graph, err := h.service.GetTicketGraph(c.Request().Context(), projectID, userID, options)
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
	options := TicketListOptions{Limit: limit, Offset: offset}
	return ticketFilterOptions(c, options)
}

// graphOptions parses bounded graph filters without pagination offsets.
func graphOptions(c echo.Context) (TicketListOptions, error) {
	limit, err := parseOptionalPositiveInt(c.QueryParam("limit"))
	if err != nil {
		return TicketListOptions{}, ErrInvalidTicketInput
	}
	options := TicketListOptions{Limit: limit}
	return ticketFilterOptions(c, options)
}

// ticketFilterOptions parses optional ticket metadata and assignment filters.
func ticketFilterOptions(c echo.Context, options TicketListOptions) (TicketListOptions, error) {
	options.Status = strings.TrimSpace(c.QueryParam("status"))
	options.Priority = strings.TrimSpace(c.QueryParam("priority"))
	options.Type = strings.TrimSpace(c.QueryParam("type"))
	options.Query = strings.TrimSpace(c.QueryParam("q"))
	assignee := strings.TrimSpace(c.QueryParam("assignee"))
	assigneeID := strings.TrimSpace(c.QueryParam("assignee_id"))
	switch {
	case assignee == "me":
		value := c.Get("userID").(string)
		options.AssigneeID = &value
	case assignee == "unassigned":
		options.Unassigned = true
	case assignee != "":
		return TicketListOptions{}, ErrInvalidTicketInput
	}
	if assigneeID != "" {
		if options.Unassigned || options.AssigneeID != nil || !validUUID(assigneeID) {
			return TicketListOptions{}, ErrInvalidTicketInput
		}
		options.AssigneeID = &assigneeID
	}
	return options, nil
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

// validateMoveTicketIDs validates optional move boundary UUIDs.
func validateMoveTicketIDs(req moveTicketRequest) error {
	if req.BeforeTicketID != nil && !validUUID(*req.BeforeTicketID) {
		return ErrInvalidTicketInput
	}
	if req.AfterTicketID != nil && !validUUID(*req.AfterTicketID) {
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

// writeSSE writes one server-sent event frame.
func writeSSE(w io.Writer, eventName, eventID string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if eventID != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", eventID); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, raw)
	return err
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
	case errors.Is(err, apperror.ErrForbidden):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrConflict):
		return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, apperror.ErrPrecondition):
		return c.JSON(http.StatusPreconditionFailed, map[string]string{"error": err.Error(), "code": "version_conflict"})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "ticket operation failed"})
	}
}
