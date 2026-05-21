package ticket

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/project"
)

// ProjectChecker exposes project access checks required by ticket workflows.
type ProjectChecker interface {
	GetProjectByID(ctx context.Context, projectID, userID string) (*project.Project, error)
	GetProjectRole(ctx context.Context, projectID, userID string) (string, error)
}

// Service contains ticket, assignment, and dependency workflows.
type Service struct {
	repo           Repository
	projectService ProjectChecker
	apConfig       activitypub.Config
	delivery       DeliveryEnqueuer
}

// DeliveryEnqueuer queues federation deliveries created by ticket actions.
type DeliveryEnqueuer interface {
	Enqueue(ctx context.Context, activityID string, targetInboxURL string) (*apdelivery.Delivery, error)
	EnqueueProjectFollowers(ctx context.Context, projectID string, activityIDs ...string) error
	EnqueueProjectTicketRecipients(ctx context.Context, projectID string, ticketID string, activityIDs ...string) error
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

// GetProjectRole returns a user's role in a project through the project service.
func (s *Service) GetProjectRole(ctx context.Context, projectID, userID string) (string, error) {
	return s.projectService.GetProjectRole(ctx, projectID, userID)
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

// ErrInvalidTicketInput reports malformed ticket-management input.
var ErrInvalidTicketInput = errors.New("invalid ticket input")

// ticketRanks defines allowed parent-child ordering for ticket hierarchy.
var ticketRanks = map[string]int{
	"epic":    3,
	"task":    2,
	"subtask": 1,
}

var (
	ticketPriorities = map[string]bool{
		"low":    true,
		"medium": true,
		"high":   true,
		"urgent": true,
	}
	ticketStatuses = map[string]bool{
		"open":        true,
		"in_progress": true,
		"review":      true,
		"done":        true,
	}
)

// CreateTicket creates a ticket and records its ActivityPub Create activity.
func (s *Service) CreateTicket(ctx context.Context, req CreateTicketRequest, projectID, reporterID string) (*Ticket, error) {
	if err := s.requireProjectPermission(ctx, projectID, reporterID, project.CanWriteTickets, "insufficient permissions: viewers cannot create tickets"); err != nil {
		return nil, err
	}

	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		return nil, invalidTicketInput("title is required")
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

	activityIDs, err := s.repo.Create(ctx, t)
	if err != nil {
		return nil, err
	}
	s.enqueueProjectTicketRecipients(ctx, projectID, t.ID, activityIDs...)

	return t, nil
}

// ListTicketsInProject returns tickets in a project visible to the user.
func (s *Service) ListTicketsInProject(ctx context.Context, projectID, userID string) ([]Ticket, error) {
	_, err := s.projectService.GetProjectByID(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}

	return s.repo.ListByProjectID(ctx, projectID)
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
	if err := s.requireProjectPermission(ctx, ticketToUpdate.ProjectID, userID, project.CanWriteTickets, "insufficient permissions: viewers cannot update tickets"); err != nil {
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
		trimmedTitle := strings.TrimSpace(*req.Title)
		if trimmedTitle == "" {
			return invalidTicketInput("title is required")
		}
		req.Title = &trimmedTitle
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

	activityIDs, err := s.repo.Update(ctx, ticketToUpdate, userID)
	if err != nil {
		return err
	}
	s.enqueueProjectTicketRecipients(ctx, ticketToUpdate.ProjectID, ticketToUpdate.ID, activityIDs...)
	return nil
}

// invalidTicketInput wraps a validation message with the ticket input sentinel.
func invalidTicketInput(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidTicketInput, message)
}

// DeleteTicket removes a ticket and tombstones its ActivityPub objects.
func (s *Service) DeleteTicket(ctx context.Context, ticketID, userID string) error {
	ticket, err := s.GetTicketByID(ctx, ticketID, userID)
	if err != nil {
		return err
	}
	if err := s.requireProjectPermission(ctx, ticket.ProjectID, userID, project.CanDeleteTickets, "insufficient permissions: only owners or managers can delete tickets"); err != nil {
		return err
	}

	deleteResult, err := s.repo.Delete(ctx, ticketID, userID)
	if err != nil {
		return err
	}
	s.enqueueRecipientInboxes(ctx, ticket.ProjectID, deleteResult)
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
	if err := s.requireProjectPermission(ctx, source.ProjectID, userID, project.CanWriteTickets, "insufficient permissions: viewers cannot update ticket links"); err != nil {
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

	return s.repo.CreateLink(ctx, link)
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

// enqueueProjectFollowers queues ticket activities to all remote project followers.
func (s *Service) enqueueProjectFollowers(ctx context.Context, projectID string, activityIDs ...string) {
	if s.delivery == nil || len(activityIDs) == 0 {
		return
	}
	if err := s.delivery.EnqueueProjectFollowers(ctx, projectID, activityIDs...); err != nil {
		log.Printf("failed to enqueue ActivityPub deliveries for project %s: %v", projectID, err)
	}
}

// enqueueProjectTicketRecipients queues ticket activities to ticket-related recipients.
func (s *Service) enqueueProjectTicketRecipients(ctx context.Context, projectID, ticketID string, activityIDs ...string) {
	if s.delivery == nil || len(activityIDs) == 0 {
		return
	}
	if err := s.delivery.EnqueueProjectTicketRecipients(ctx, projectID, ticketID, activityIDs...); err != nil {
		log.Printf("failed to enqueue ActivityPub deliveries for project %s ticket %s: %v", projectID, ticketID, err)
	}
}

// enqueueRecipientInboxes queues delete activities to precomputed remote inboxes.
func (s *Service) enqueueRecipientInboxes(ctx context.Context, projectID string, result *DeleteResult) {
	if s.delivery == nil || result == nil || len(result.ActivityIDs) == 0 {
		return
	}
	for _, activityID := range result.ActivityIDs {
		if activityID == "" {
			continue
		}
		for _, inbox := range result.RecipientInboxes {
			if inbox == "" {
				continue
			}
			if _, err := s.delivery.Enqueue(ctx, activityID, inbox); err != nil {
				log.Printf("failed to enqueue ActivityPub delivery for project %s inbox %s: %v", projectID, inbox, err)
			}
		}
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
	if err := s.requireProjectPermission(ctx, source.ProjectID, userID, project.CanWriteTickets, "insufficient permissions: viewers cannot update ticket links"); err != nil {
		return err
	}
	return s.repo.DeleteLink(ctx, linkID)
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
	Nodes []GraphNode `json:"nodes"`
	Links []GraphLink `json:"links"`
}

// GetTicketGraph returns project tickets and links as graph data.
func (s *Service) GetTicketGraph(ctx context.Context, projectID, userID string) (*GraphResponse, error) {
	_, err := s.projectService.GetProjectByID(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}

	tickets, err := s.repo.ListByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	links, err := s.repo.GetLinksByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	response := &GraphResponse{
		Nodes: make([]GraphNode, 0, len(tickets)),
		Links: make([]GraphLink, 0, len(links)+len(tickets)),
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
			response.Links = append(response.Links, GraphLink{
				Source: *t.ParentID,
				Target: t.ID,
				Type:   "hierarchy",
			})
		}
	}

	for _, l := range links {
		response.Links = append(response.Links, GraphLink{
			Source: l.SourceID,
			Target: l.TargetID,
			Type:   l.LinkType,
		})
	}

	return response, nil
}

// requireProjectPermission checks a project role predicate and normalizes denial errors.
func (s *Service) requireProjectPermission(ctx context.Context, projectID, userID string, allowed func(string) bool, deniedMessage string) error {
	role, err := s.projectService.GetProjectRole(ctx, projectID, userID)
	if err != nil {
		return errors.New("project not found or access denied")
	}
	if !allowed(role) {
		return errors.New(deniedMessage)
	}
	return nil
}
