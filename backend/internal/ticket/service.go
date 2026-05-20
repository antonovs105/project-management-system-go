package ticket

import (
	"context"
	"errors"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/project"
)

// ProjectChecker interface
type ProjectChecker interface {
	GetProjectByID(ctx context.Context, projectID, userID string) (*project.Project, error)
}

type Service struct {
	repo           Repository
	projectService ProjectChecker
	apConfig       activitypub.Config
}

func NewService(repo Repository, projectService ProjectChecker, apConfig activitypub.Config) *Service {
	return &Service{
		repo:           repo,
		projectService: projectService,
		apConfig:       apConfig,
	}
}

// CreateTicketRequest DTO for ticket creation
type CreateTicketRequest struct {
	Title       string
	Description string
	Priority    string
	Type        string
	ParentID    *string
	AssigneeID  *string
}

// Hierarchy ranks
var ticketRanks = map[string]int{
	"epic":    3,
	"task":    2,
	"subtask": 1,
}

// CreateTicket logic for ticket creation
func (s *Service) CreateTicket(ctx context.Context, req CreateTicketRequest, projectID, reporterID string) (*Ticket, error) {
	// checking access
	_, err := s.projectService.GetProjectByID(ctx, projectID, reporterID)
	if err != nil {
		return nil, err
	}

	// Validate Ticket Type
	rank, ok := ticketRanks[req.Type]
	if !ok {
		if req.Type == "" {
			req.Type = "task"
			rank = 2
		} else {
			return nil, errors.New("invalid ticket type")
		}
	}

	// Validate Hierarchy
	if req.ParentID != nil {
		parent, err := s.repo.GetByID(ctx, *req.ParentID)
		if err != nil {
			return nil, errors.New("parent ticket not found")
		}
		if parent.ProjectID != projectID {
			return nil, errors.New("parent ticket must be in the same project")
		}

		parentRank, ok := ticketRanks[parent.Type]
		if !ok {
			parentRank = 2
		}

		if parentRank <= rank {
			return nil, errors.New("invalid hierarchy: parent must be of higher rank (Epic > Task > Subtask)")
		}
	} else {
		if req.Type == "subtask" {
			return nil, errors.New("subtask must have a parent")
		}
	}

	// TODO: check is AssigneeID a project member
	if req.Priority == "" {
		req.Priority = "medium"
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

	err = s.repo.Create(ctx, t)
	if err != nil {
		return nil, err
	}

	return t, nil
}

// ListTicketsInProject logic for ticket list
func (s *Service) ListTicketsInProject(ctx context.Context, projectID, userID string) ([]Ticket, error) {
	// check access
	_, err := s.projectService.GetProjectByID(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}

	return s.repo.ListByProjectID(ctx, projectID)
}

// GetTicketByID gogic to get single ticket
func (s *Service) GetTicketByID(ctx context.Context, ticketID, userID string) (*Ticket, error) {
	ticket, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, errors.New("ticket not found")
	}

	// check access
	_, err = s.projectService.GetProjectByID(ctx, ticket.ProjectID, userID)
	if err != nil {
		return nil, errors.New("ticket not found or access denied")
	}

	return ticket, nil
}

// UpdateTicketRequest DTO for updating ticket
type UpdateTicketRequest struct {
	Title       *string  `json:"title"`
	Description *string  `json:"description"`
	Status      *string  `json:"status"`
	Priority    *string  `json:"priority"`
	Type        *string  `json:"type"`
	ParentID    **string `json:"parent_id"`
	AssigneeID  **string `json:"assignee_id"`
}

// UpdateTicket logic for update
func (s *Service) UpdateTicket(ctx context.Context, req UpdateTicketRequest, ticketID, userID string) error {
	// find ticket, check access
	ticketToUpdate, err := s.GetTicketByID(ctx, ticketID, userID)
	if err != nil {
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
			return errors.New("invalid ticket type")
		}

		if newParentID != nil {
			parent, err := s.repo.GetByID(ctx, *newParentID)
			if err != nil {
				return errors.New("parent ticket not found")
			}
			if parent.ProjectID != ticketToUpdate.ProjectID {
				return errors.New("parent ticket must be in the same project")
			}
			if parent.ID == ticketToUpdate.ID {
				return errors.New("cannot be own parent")
			}

			// parent rank check
			parentRank := ticketRanks[parent.Type]
			if parentRank <= rank {
				return errors.New("invalid hierarchy: parent must be of higher rank")
			}
		} else {
			if newType == "subtask" {
				return errors.New("subtask must have a parent")
			}
		}
	}

	// TODO: add more advanced check

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

	return s.repo.Update(ctx, ticketToUpdate)
}

// DeleteTicket logic for deleting
func (s *Service) DeleteTicket(ctx context.Context, ticketID, userID string) error {
	// check access
	_, err := s.GetTicketByID(ctx, ticketID, userID)
	if err != nil {
		return err
	}

	return s.repo.Delete(ctx, ticketID)
}

// AddTicketLink adds a link and checks for cycles
func (s *Service) AddTicketLink(ctx context.Context, sourceID, targetID string, linkType string, projectID, userID string) error {
	if sourceID == targetID {
		return errors.New("cannot link ticket to itself")
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
		return errors.New("cannot link tickets from different projects")
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
		return errors.New("cycle detected: path already exists from target to source")
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

// RemoveTicketLink removes a link
func (s *Service) RemoveTicketLink(ctx context.Context, linkID, projectID, userID string) error {

	err := s.repo.DeleteLink(ctx, linkID)
	return err
}

// GraphNode DTO
type GraphNode struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Group    string `json:"group"`
}

// GraphLink DTO
type GraphLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

// GraphResponse DTO
type GraphResponse struct {
	Nodes []GraphNode `json:"nodes"`
	Links []GraphLink `json:"links"`
}

// GetTicketGraph returns nodes and links for react-force-graph
func (s *Service) GetTicketGraph(ctx context.Context, projectID, userID string) (*GraphResponse, error) {
	// check access
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
