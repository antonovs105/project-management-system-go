package ticket

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/project"
)

// ProjectChecker exposes project access checks required by ticket workflows.
type ProjectChecker interface {
	GetProjectByID(ctx context.Context, projectID, userID string) (*project.Project, error)
	GetProjectRole(ctx context.Context, projectID, userID string) (string, error)
	HasProjectPermission(ctx context.Context, projectID, userID, permission string) (bool, error)
}

// Service contains ticket, assignment, and dependency workflows.
type Service struct {
	repo           Repository
	projectService ProjectChecker
	apConfig       activitypub.Config
	delivery       DeliveryEnqueuer
	events         EventPublisher
	notifications  NotificationSink
}

// DeliveryEnqueuer queues federation deliveries created by ticket actions.
type DeliveryEnqueuer interface {
	EnqueuePersisted(ctx context.Context, deliveries []apdelivery.QueueCandidate) error
}

// NotificationSink receives local user notifications caused by ticket workflows.
type NotificationSink interface {
	NotifyTicketAssigned(ctx context.Context, assigneeID, actorID string, ticket Ticket) error
}

// NewService creates a ticket service.
func NewService(repo Repository, projectService ProjectChecker, apConfig activitypub.Config) *Service {
	return &Service{
		repo:           repo,
		projectService: projectService,
		apConfig:       apConfig,
	}
}

// SetDelivery attaches the delivery queue used for ticket federation.
func (s *Service) SetDelivery(delivery DeliveryEnqueuer) {
	s.delivery = delivery
}

// SetEventPublisher attaches the realtime event publisher used by ticket workflows.
func (s *Service) SetEventPublisher(events EventPublisher) {
	s.events = events
}

// SetNotificationSink attaches the local notification sink used by assignment workflows.
func (s *Service) SetNotificationSink(notifications NotificationSink) {
	s.notifications = notifications
}

// GetProjectRole returns a user's role in a project through the project service.
func (s *Service) GetProjectRole(ctx context.Context, projectID, userID string) (string, error) {
	return s.projectService.GetProjectRole(ctx, projectID, userID)
}

// HasProjectPermission returns a user's project permission through the project service.
func (s *Service) HasProjectPermission(ctx context.Context, projectID, userID, permission string) (bool, error) {
	return s.projectService.HasProjectPermission(ctx, projectID, userID, permission)
}

// CreateTicketRequest contains fields for creating a ticket.
type CreateTicketRequest struct {
	Title       string
	Description string
	Priority    string
	Type        string
	ParentID    *string
	AssigneeID  *string
}

// MoveTicketRequest contains the target board position for a ticket.
type MoveTicketRequest struct {
	Status         string
	BeforeTicketID *string
	AfterTicketID  *string
}

// ErrInvalidTicketInput reports malformed ticket-management input.
var ErrInvalidTicketInput = errors.New("invalid ticket input")

const (
	// defaultTicketListLimit is the fallback ticket list size.
	defaultTicketListLimit = 100
	// maxTicketListLimit is the largest accepted ticket list size.
	maxTicketListLimit = 500
	// defaultGraphNodeLimit is the fallback graph node set returned to the browser.
	defaultGraphNodeLimit = 250
	// maxGraphNodeLimit is the largest graph node set returned to the browser.
	maxGraphNodeLimit = 500
	// maxTicketTitleLength is the longest accepted ticket title.
	maxTicketTitleLength = 120
	// maxTicketDescriptionLength is the longest accepted ticket description.
	maxTicketDescriptionLength = 4000
)

// ticketRanks defines allowed parent-child ordering for ticket hierarchy.
var ticketRanks = map[string]int{
	"epic":    3,
	"task":    2,
	"subtask": 1,
}

var (
	// ticketPriorities lists accepted ticket priority values.
	ticketPriorities = map[string]bool{
		"low":    true,
		"medium": true,
		"high":   true,
		"urgent": true,
	}
	// ticketStatuses lists accepted local workflow status values.
	ticketStatuses = map[string]bool{
		"open":        true,
		"in_progress": true,
		"review":      true,
		"done":        true,
	}
)

// CreateTicket creates a ticket and records its ActivityPub Create activity.
func (s *Service) CreateTicket(ctx context.Context, req CreateTicketRequest, projectID, reporterID string) (*Ticket, error) {
	if err := s.requireProjectPermission(ctx, projectID, reporterID, project.PermissionTicketsCreate, "insufficient permissions: missing tickets.create"); err != nil {
		return nil, err
	}

	var err error
	req.Title, err = normalizeRequiredTicketText(req.Title, "title", maxTicketTitleLength)
	if err != nil {
		return nil, err
	}
	req.Description, err = normalizeOptionalTicketText(req.Description, "description", maxTicketDescriptionLength)
	if err != nil {
		return nil, err
	}

	rank, ok := ticketRanks[req.Type]
	if !ok {
		if req.Type == "" {
			req.Type = "task"
			rank = 2
		} else {
			return nil, invalidTicketInput("invalid ticket type")
		}
	}

	if req.ParentID != nil {
		parent, err := s.repo.GetByID(ctx, *req.ParentID)
		if err != nil {
			return nil, invalidTicketInput("parent ticket not found")
		}
		if parent.ProjectID != projectID {
			return nil, invalidTicketInput("parent ticket must be in the same project")
		}

		parentRank, ok := ticketRanks[parent.Type]
		if !ok {
			parentRank = 2
		}

		if parentRank <= rank {
			return nil, invalidTicketInput("invalid hierarchy: parent must be of higher rank (Epic > Task > Subtask)")
		}
	} else {
		if req.Type == "subtask" {
			return nil, invalidTicketInput("subtask must have a parent")
		}
	}

	if req.Priority == "" {
		req.Priority = "medium"
	}
	if !ticketPriorities[req.Priority] {
		return nil, invalidTicketInput("invalid ticket priority")
	}

	ticketID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}

	t := &Ticket{
		ID:          ticketID,
		APID:        activitypub.TicketAPID(s.apConfig, ticketID),
		Title:       req.Title,
		Description: req.Description,
		Status:      "open",
		Priority:    req.Priority,
		Type:        req.Type,
		ParentID:    req.ParentID,
		ProjectID:   projectID,
		ReporterID:  reporterID,
		AssigneeID:  req.AssigneeID,
	}

	result, err := s.repo.Create(ctx, t)
	if err != nil {
		return nil, err
	}
	s.enqueueDeliveries(ctx, projectID, result.Deliveries)
	s.publishTicketEvent(Event{Type: EventTicketCreated, ProjectID: projectID, TicketID: t.ID})
	s.notifyTicketAssigned(ctx, reporterID, t)

	return t, nil
}

// ListTicketsInProject returns tickets in a project visible to the user.
func (s *Service) ListTicketsInProject(ctx context.Context, projectID, userID string, options TicketListOptions) ([]Ticket, error) {
	_, err := s.projectService.GetProjectByID(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}

	options, err = normalizeTicketListOptions(options)
	if err != nil {
		return nil, err
	}
	return s.repo.ListByProjectID(ctx, projectID, options)
}

// GetTicketByID returns a single ticket visible to the user.
func (s *Service) GetTicketByID(ctx context.Context, ticketID, userID string) (*Ticket, error) {
	ticket, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, errors.New("ticket not found")
	}

	_, err = s.projectService.GetProjectByID(ctx, ticket.ProjectID, userID)
	if err != nil {
		return nil, errors.New("ticket not found or access denied")
	}

	return ticket, nil
}

// UpdateTicketRequest contains nullable partial ticket updates.
type UpdateTicketRequest struct {
	Title       *string  `json:"title"`
	Description *string  `json:"description"`
	Status      *string  `json:"status"`
	Priority    *string  `json:"priority"`
	Type        *string  `json:"type"`
	ParentID    **string `json:"parent_id"`
	AssigneeID  **string `json:"assignee_id"`
}

// UpdateTicket changes ticket fields and emits Update or Add activities as needed.
func (s *Service) UpdateTicket(ctx context.Context, req UpdateTicketRequest, ticketID, userID string) error {
	ticketToUpdate, err := s.GetTicketByID(ctx, ticketID, userID)
	if err != nil {
		return err
	}
	previousAssigneeID := ticketToUpdate.AssigneeID
	if err := s.requireProjectPermission(ctx, ticketToUpdate.ProjectID, userID, project.PermissionTicketsUpdate, "insufficient permissions: missing tickets.update"); err != nil {
		return err
	}

	// New values or keep old
	newType := ticketToUpdate.Type
	if req.Type != nil {
		newType = *req.Type
	}

	newParentID := ticketToUpdate.ParentID
	if req.ParentID != nil {
		newParentID = *req.ParentID
	}

	// Hierarchy Validation if Type or ParentID changes
	if req.Type != nil || req.ParentID != nil {
		rank, ok := ticketRanks[newType]
		if !ok {
			return invalidTicketInput("invalid ticket type")
		}

		if newParentID != nil {
			parent, err := s.repo.GetByID(ctx, *newParentID)
			if err != nil {
				return invalidTicketInput("parent ticket not found")
			}
			if parent.ProjectID != ticketToUpdate.ProjectID {
				return invalidTicketInput("parent ticket must be in the same project")
			}
			if parent.ID == ticketToUpdate.ID {
				return invalidTicketInput("cannot be own parent")
			}

			// parent rank check
			parentRank := ticketRanks[parent.Type]
			if parentRank <= rank {
				return invalidTicketInput("invalid hierarchy: parent must be of higher rank")
			}
		} else {
			if newType == "subtask" {
				return invalidTicketInput("subtask must have a parent")
			}
		}
	}

	if req.Title != nil {
		trimmedTitle, err := normalizeRequiredTicketText(*req.Title, "title", maxTicketTitleLength)
		if err != nil {
			return err
		}
		req.Title = &trimmedTitle
	}
	if req.Description != nil {
		description, err := normalizeOptionalTicketText(*req.Description, "description", maxTicketDescriptionLength)
		if err != nil {
			return err
		}
		req.Description = &description
	}
	if req.Status != nil && !ticketStatuses[*req.Status] {
		return invalidTicketInput("invalid ticket status")
	}
	if req.Priority != nil && !ticketPriorities[*req.Priority] {
		return invalidTicketInput("invalid ticket priority")
	}

	// update rows
	if req.Title != nil {
		ticketToUpdate.Title = *req.Title
	}
	if req.Description != nil {
		ticketToUpdate.Description = *req.Description
	}
	if req.Status != nil {
		ticketToUpdate.Status = *req.Status
	}
	if req.Priority != nil {
		ticketToUpdate.Priority = *req.Priority
	}
	if req.Type != nil {
		ticketToUpdate.Type = *req.Type
	}
	// Logic for nullable fields
	if req.ParentID != nil {
		ticketToUpdate.ParentID = *req.ParentID
	}
	if req.AssigneeID != nil {
		ticketToUpdate.AssigneeID = *req.AssigneeID
	}

	result, err := s.repo.Update(ctx, ticketToUpdate, userID)
	if err != nil {
		return err
	}
	s.enqueueDeliveries(ctx, ticketToUpdate.ProjectID, result.Deliveries)
	s.publishTicketEvent(Event{Type: EventTicketUpdated, ProjectID: ticketToUpdate.ProjectID, TicketID: ticketToUpdate.ID})
	if assigneeChanged(previousAssigneeID, ticketToUpdate.AssigneeID) {
		s.notifyTicketAssigned(ctx, userID, ticketToUpdate)
	}
	return nil
}

// MoveTicket reorders a ticket within or across board status groups.
func (s *Service) MoveTicket(ctx context.Context, req MoveTicketRequest, ticketID, userID string) (*Ticket, error) {
	ticketToMove, err := s.GetTicketByID(ctx, ticketID, userID)
	if err != nil {
		return nil, err
	}
	if err := s.requireProjectPermission(ctx, ticketToMove.ProjectID, userID, project.PermissionTicketsUpdate, "insufficient permissions: missing tickets.update"); err != nil {
		return nil, err
	}
	if req.Status == "" {
		req.Status = ticketToMove.Status
	}
	if !ticketStatuses[req.Status] {
		return nil, invalidTicketInput("invalid ticket status")
	}
	moved, result, err := s.repo.Move(ctx, ticketID, userID, req.Status, req.BeforeTicketID, req.AfterTicketID)
	if err != nil {
		return nil, err
	}
	s.enqueueDeliveries(ctx, moved.ProjectID, result.Deliveries)
	s.publishTicketEvent(Event{Type: EventTicketUpdated, ProjectID: moved.ProjectID, TicketID: moved.ID})
	return moved, nil
}

// invalidTicketInput wraps a validation message with the ticket input sentinel.
func invalidTicketInput(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidTicketInput, message)
}

// normalizeRequiredTicketText trims text and enforces ticket-service length limits.
func normalizeRequiredTicketText(value, label string, maxLength int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", invalidTicketInput(label + " is required")
	}
	return normalizeOptionalTicketText(trimmed, label, maxLength)
}

// normalizeOptionalTicketText trims text and rejects unexpectedly large values.
func normalizeOptionalTicketText(value, label string, maxLength int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if utf8.RuneCountInString(trimmed) > maxLength {
		return "", invalidTicketInput(fmt.Sprintf("%s must be at most %d characters", label, maxLength))
	}
	return trimmed, nil
}

// normalizeTicketListOptions validates filters and bounds pagination.
func normalizeTicketListOptions(options TicketListOptions) (TicketListOptions, error) {
	var err error
	options, err = normalizeTicketFilters(options)
	if err != nil {
		return TicketListOptions{}, err
	}
	options.Limit = normalizeTicketListLimit(options.Limit)
	options.Offset = normalizeTicketListOffset(options.Offset)
	return options, nil
}

// normalizeTicketFilters validates optional ticket metadata filters.
func normalizeTicketFilters(options TicketListOptions) (TicketListOptions, error) {
	options.Status = strings.ToLower(strings.TrimSpace(options.Status))
	if options.Status != "" && !ticketStatuses[options.Status] {
		return TicketListOptions{}, invalidTicketInput("invalid ticket status")
	}
	options.Priority = strings.ToLower(strings.TrimSpace(options.Priority))
	if options.Priority != "" && !ticketPriorities[options.Priority] {
		return TicketListOptions{}, invalidTicketInput("invalid ticket priority")
	}
	options.Type = strings.ToLower(strings.TrimSpace(options.Type))
	if options.Type != "" {
		if _, ok := ticketRanks[options.Type]; !ok {
			return TicketListOptions{}, invalidTicketInput("invalid ticket type")
		}
	}
	return options, nil
}

// normalizeTicketListLimit bounds ticket list sizes.
func normalizeTicketListLimit(limit int) int {
	if limit <= 0 {
		return defaultTicketListLimit
	}
	if limit > maxTicketListLimit {
		return maxTicketListLimit
	}
	return limit
}

// normalizeGraphOptions validates graph filters and bounds node fanout.
func normalizeGraphOptions(options TicketListOptions) (TicketListOptions, error) {
	var err error
	options, err = normalizeTicketFilters(options)
	if err != nil {
		return TicketListOptions{}, err
	}
	options.Offset = 0
	if options.Limit <= 0 {
		options.Limit = defaultGraphNodeLimit
	}
	if options.Limit > maxGraphNodeLimit {
		options.Limit = maxGraphNodeLimit
	}
	return options, nil
}

// normalizeTicketListOffset clamps negative ticket list offsets.
func normalizeTicketListOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

// DeleteTicket removes a ticket and tombstones its ActivityPub objects.
func (s *Service) DeleteTicket(ctx context.Context, ticketID, userID string) error {
	ticket, err := s.GetTicketByID(ctx, ticketID, userID)
	if err != nil {
		return err
	}
	if err := s.requireProjectPermission(ctx, ticket.ProjectID, userID, project.PermissionTicketsDelete, "insufficient permissions: missing tickets.delete"); err != nil {
		return err
	}

	deleteResult, err := s.repo.Delete(ctx, ticketID, userID)
	if err != nil {
		return err
	}
	s.enqueueDeliveries(ctx, ticket.ProjectID, deleteResult.Deliveries)
	s.publishTicketEvent(Event{Type: EventTicketDeleted, ProjectID: ticket.ProjectID, TicketID: ticket.ID})
	return nil
}

// AddTicketLink creates a directed ticket link after checking for cycles.
func (s *Service) AddTicketLink(ctx context.Context, sourceID, targetID string, linkType string, projectID, userID string) error {
	if sourceID == targetID {
		return invalidTicketInput("cannot link ticket to itself")
	}
	linkType = strings.TrimSpace(linkType)
	if linkType == "" {
		return invalidTicketInput("link type is required")
	}

	// check access and existence
	source, err := s.GetTicketByID(ctx, sourceID, userID)
	if err != nil {
		return err
	}
	target, err := s.GetTicketByID(ctx, targetID, userID)
	if err != nil {
		return err
	}

	if source.ProjectID != target.ProjectID {
		return invalidTicketInput("cannot link tickets from different projects")
	}
	if err := s.requireProjectPermission(ctx, source.ProjectID, userID, project.PermissionTicketsUpdate, "insufficient permissions: missing tickets.update"); err != nil {
		return err
	}

	// Cycle Detection
	// Get all links in the project to build the graph
	links, err := s.repo.GetLinksByProjectID(ctx, source.ProjectID)
	if err != nil {
		return err
	}

	// Build adjacency list
	adj := make(map[string][]string)
	for _, l := range links {
		adj[l.SourceID] = append(adj[l.SourceID], l.TargetID)
	}

	// Check if path exists from targetID to sourceID
	if hasPath(adj, targetID, sourceID) {
		return invalidTicketInput("cycle detected: path already exists from target to source")
	}

	// Create Link
	link := &TicketLink{
		SourceID: sourceID,
		TargetID: targetID,
		LinkType: linkType,
	}

	if err := s.repo.CreateLink(ctx, link); err != nil {
		return err
	}
	s.publishTicketEvent(Event{Type: EventTicketLinked, ProjectID: source.ProjectID, TicketID: source.ID})
	return nil
}

// hasPath checks if there is a path from start to end using BFS
func hasPath(adj map[string][]string, start, end string) bool {
	visited := make(map[string]bool)
	queue := []string{start}
	visited[start] = true

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr == end {
			return true
		}

		for _, neighbor := range adj[curr] {
			if !visited[neighbor] {
				visited[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}
	return false
}

// enqueueDeliveries queues delivery rows created in the ticket transaction.
func (s *Service) enqueueDeliveries(ctx context.Context, projectID string, deliveries []apdelivery.QueueCandidate) {
	if s.delivery == nil || len(deliveries) == 0 {
		return
	}
	if err := s.delivery.EnqueuePersisted(ctx, deliveries); err != nil {
		log.Printf("failed to enqueue ActivityPub deliveries for project %s: %v", projectID, err)
	}
}

// RemoveTicketLink removes a ticket link when the user can edit the source ticket.
func (s *Service) RemoveTicketLink(ctx context.Context, linkID, projectID, userID string) error {
	link, err := s.repo.GetLinkByID(ctx, linkID)
	if err != nil {
		return err
	}
	source, err := s.GetTicketByID(ctx, link.SourceID, userID)
	if err != nil {
		return err
	}
	if err := s.requireProjectPermission(ctx, source.ProjectID, userID, project.PermissionTicketsUpdate, "insufficient permissions: missing tickets.update"); err != nil {
		return err
	}
	if err := s.repo.DeleteLink(ctx, linkID); err != nil {
		return err
	}
	s.publishTicketEvent(Event{Type: EventTicketUnlinked, ProjectID: source.ProjectID, TicketID: source.ID})
	return nil
}

// GraphNode is a ticket node in the graph response.
type GraphNode struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Group    string `json:"group"`
}

// GraphLink is a ticket dependency or hierarchy edge in the graph response.
type GraphLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

// GraphResponse contains graph-ready ticket nodes and links.
type GraphResponse struct {
	Nodes     []GraphNode `json:"nodes"`
	Links     []GraphLink `json:"links"`
	Limit     int         `json:"limit"`
	Truncated bool        `json:"truncated"`
}

// GetTicketGraph returns project tickets and links as graph data.
func (s *Service) GetTicketGraph(ctx context.Context, projectID, userID string, options TicketListOptions) (*GraphResponse, error) {
	_, err := s.projectService.GetProjectByID(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	options, err = normalizeGraphOptions(options)
	if err != nil {
		return nil, err
	}

	queryOptions := options
	queryOptions.Limit = options.Limit + 1
	tickets, err := s.repo.ListByProjectID(ctx, projectID, queryOptions)
	if err != nil {
		return nil, err
	}
	truncated := len(tickets) > options.Limit
	if truncated {
		tickets = tickets[:options.Limit]
	}

	links, err := s.repo.GetLinksByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	response := &GraphResponse{
		Nodes:     make([]GraphNode, 0, len(tickets)),
		Links:     make([]GraphLink, 0, len(links)+len(tickets)),
		Limit:     options.Limit,
		Truncated: truncated,
	}

	nodeIDs := make(map[string]struct{}, len(tickets))
	for _, t := range tickets {
		nodeIDs[t.ID] = struct{}{}
	}

	for _, t := range tickets {
		response.Nodes = append(response.Nodes, GraphNode{
			ID:       t.ID,
			Label:    t.Title,
			Type:     t.Type,
			Status:   t.Status,
			Priority: t.Priority,
			Group:    t.Type,
		})

		// Add implicit hierarchy links
		if t.ParentID != nil {
			if _, ok := nodeIDs[*t.ParentID]; ok {
				response.Links = append(response.Links, GraphLink{
					Source: *t.ParentID,
					Target: t.ID,
					Type:   "hierarchy",
				})
			}
		}
	}

	for _, l := range links {
		if _, ok := nodeIDs[l.SourceID]; !ok {
			continue
		}
		if _, ok := nodeIDs[l.TargetID]; !ok {
			continue
		}
		response.Links = append(response.Links, GraphLink{
			Source: l.SourceID,
			Target: l.TargetID,
			Type:   l.LinkType,
		})
	}

	return response, nil
}

// requireProjectPermission checks a project permission and normalizes denial errors.
func (s *Service) requireProjectPermission(ctx context.Context, projectID, userID string, permission string, deniedMessage string) error {
	allowed, err := s.projectService.HasProjectPermission(ctx, projectID, userID, permission)
	if err != nil {
		return errors.New("project not found or access denied")
	}
	if !allowed {
		return errors.New(deniedMessage)
	}
	return nil
}

// publishTicketEvent emits a realtime ticket event when a publisher is attached.
func (s *Service) publishTicketEvent(event Event) {
	if s.events == nil {
		return
	}
	s.events.PublishTicketEvent(event)
}

// notifyTicketAssigned creates a local notification for newly assigned users.
func (s *Service) notifyTicketAssigned(ctx context.Context, actorID string, ticket *Ticket) {
	if s.notifications == nil || ticket == nil || ticket.AssigneeID == nil || *ticket.AssigneeID == "" || *ticket.AssigneeID == actorID {
		return
	}
	if err := s.notifications.NotifyTicketAssigned(ctx, *ticket.AssigneeID, actorID, *ticket); err != nil {
		log.Printf("failed to create assignment notification for ticket %s assignee %s: %v", ticket.ID, *ticket.AssigneeID, err)
	}
}

// assigneeChanged reports whether a single-assignee value changed.
func assigneeChanged(before, after *string) bool {
	switch {
	case before == nil && after == nil:
		return false
	case before == nil || after == nil:
		return true
	default:
		return *before != *after
	}
}
