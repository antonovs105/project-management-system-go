package portability

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/antonovs105/project-management-system-go/internal/comment"
	"github.com/antonovs105/project-management-system-go/internal/label"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/antonovs105/project-management-system-go/internal/user"
)

// ErrInvalidBundle reports an unsupported or unsafe import bundle.
var ErrInvalidBundle = errors.New("invalid portability bundle")

const (
	maxImportLabels   = 200
	maxImportTickets  = 5000
	maxImportComments = 20000
)

var portableLabelColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// ProjectService is the project workflow subset needed for data transfer.
type ProjectService interface {
	CreateProject(context.Context, string, string, string) (*project.Project, error)
	GetProjectByID(context.Context, string, string) (*project.Project, error)
	ListUserProjects(context.Context, string, project.ProjectListOptions) ([]project.Project, error)
	ListProjectMembers(context.Context, string, string, project.ProjectListOptions) ([]project.ProjectMember, error)
}

// TicketService is the ticket workflow subset needed for data transfer.
type TicketService interface {
	CreateTicket(context.Context, ticket.CreateTicketRequest, string, string) (*ticket.Ticket, error)
	UpdateTicket(context.Context, ticket.UpdateTicketRequest, string, string) error
	ListTicketsInProject(context.Context, string, string, ticket.TicketListOptions) ([]ticket.Ticket, error)
}

// LabelService is the label workflow subset needed for data transfer.
type LabelService interface {
	List(context.Context, string, string) ([]label.Label, error)
	Create(context.Context, string, string, string, string) (*label.Label, error)
}

// CommentService is the comment workflow subset needed for data transfer.
type CommentService interface {
	ListComments(context.Context, string, string, comment.CommentListOptions) ([]comment.Comment, error)
	CreateComment(context.Context, string, string, string) (*comment.Comment, error)
}

// UserService is the self-profile workflow subset needed for account export.
type UserService interface {
	GetOwnProfile(context.Context, string) (*user.User, error)
}

// Service orchestrates versioned export and validated imports through domain services.
type Service struct {
	projects ProjectService
	tickets  TicketService
	labels   LabelService
	comments CommentService
	users    UserService
	now      func() time.Time
}

// NewService creates a portability workflow service.
func NewService(projects ProjectService, tickets TicketService, labels LabelService, comments CommentService, users UserService) *Service {
	return &Service{projects: projects, tickets: tickets, labels: labels, comments: comments, users: users, now: time.Now}
}

// ExportProject returns a stable project bundle visible to userID.
func (s *Service) ExportProject(ctx context.Context, projectID, userID string) (*ProjectBundle, error) {
	value, err := s.projects.GetProjectByID(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	members, err := s.allMembers(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	labels, err := s.labels.List(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	tickets, err := s.allTickets(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}

	labelNames := make(map[string]string, len(labels))
	exportedLabels := make([]ExportLabel, 0, len(labels))
	for _, item := range labels {
		labelNames[item.ID] = item.Name
		exportedLabels = append(exportedLabels, ExportLabel{Name: item.Name, Color: item.Color})
	}
	exportedTickets := make([]ExportTicket, 0, len(tickets))
	for _, item := range tickets {
		comments, err := s.allComments(ctx, item.ID, userID)
		if err != nil {
			return nil, err
		}
		portable := ExportTicket{
			SourceID:       item.ID,
			ParentSourceID: item.ParentID,
			Title:          item.Title,
			Description:    item.Description,
			Status:         item.Status,
			Priority:       item.Priority,
			Type:           item.Type,
			DueDate:        item.DueDate,
			Labels:         make([]string, 0, len(item.LabelIDs)),
			Comments:       make([]ExportComment, 0, len(comments)),
		}
		for _, labelID := range item.LabelIDs {
			if name := labelNames[labelID]; name != "" {
				portable.Labels = append(portable.Labels, name)
			}
		}
		for _, value := range comments {
			portable.Comments = append(portable.Comments, ExportComment{AuthorSourceID: value.AuthorID, Content: value.Content, CreatedAt: value.CreatedAt})
		}
		exportedTickets = append(exportedTickets, portable)
	}

	return &ProjectBundle{
		Schema:     ProjectSchema,
		ExportedAt: s.now().UTC(),
		Project:    ExportProject{Name: value.Name, Description: value.Description},
		Members:    exportMembers(members),
		Labels:     exportedLabels,
		Tickets:    exportedTickets,
	}, nil
}

// ExportUser returns the current account profile and accessible project bundles.
func (s *Service) ExportUser(ctx context.Context, userID string) (*UserBundle, error) {
	profile, err := s.users.GetOwnProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	projects, err := s.allUserProjects(ctx, userID)
	if err != nil {
		return nil, err
	}
	bundle := &UserBundle{
		Schema:     UserSchema,
		ExportedAt: s.now().UTC(),
		Account: ExportAccount{
			Username: profile.Username, Email: profile.Email, EmailVerified: profile.EmailVerified,
			Handle: profile.Handle, Name: profile.Name, Summary: profile.Summary, CreatedAt: profile.CreatedAt,
		},
		Projects: make([]ProjectBundle, 0, len(projects)),
	}
	for _, value := range projects {
		projectBundle, err := s.ExportProject(ctx, value.ID, userID)
		if err != nil {
			return nil, err
		}
		bundle.Projects = append(bundle.Projects, *projectBundle)
	}
	return bundle, nil
}

// ImportProject validates a bundle and creates a new project owned by userID.
func (s *Service) ImportProject(ctx context.Context, userID string, bundle ProjectBundle) (*ImportResult, error) {
	if err := ValidateProjectBundle(bundle, true); err != nil {
		return nil, err
	}
	created, err := s.projects.CreateProject(ctx, bundle.Project.Name, bundle.Project.Description, userID)
	if err != nil {
		return nil, err
	}
	result, err := s.importIntoProject(ctx, created.ID, userID, bundle.Labels, bundle.Tickets)
	if err != nil {
		return nil, fmt.Errorf("project %s was created but its import did not complete: %w", created.ID, err)
	}
	return result, nil
}

// ImportTickets validates and bulk-imports tickets into an existing project.
func (s *Service) ImportTickets(ctx context.Context, projectID, userID string, bundle ProjectBundle) (*ImportResult, error) {
	if _, err := s.projects.GetProjectByID(ctx, projectID, userID); err != nil {
		return nil, err
	}
	if err := ValidateProjectBundle(bundle, false); err != nil {
		return nil, err
	}
	return s.importIntoProject(ctx, projectID, userID, bundle.Labels, bundle.Tickets)
}

func (s *Service) importIntoProject(ctx context.Context, projectID, userID string, labels []ExportLabel, tickets []ExportTicket) (*ImportResult, error) {
	result := &ImportResult{ProjectID: projectID}
	labelIDs := make(map[string]string, len(labels))
	existingLabels, err := s.labels.List(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	for _, value := range existingLabels {
		labelIDs[strings.ToLower(value.Name)] = value.ID
	}
	for _, value := range labels {
		key := strings.ToLower(value.Name)
		if labelIDs[key] != "" {
			continue
		}
		created, err := s.labels.Create(ctx, projectID, userID, value.Name, value.Color)
		if err != nil {
			return nil, err
		}
		labelIDs[key] = created.ID
		result.LabelsImported++
	}

	createdIDs := make(map[string]string, len(tickets))
	pending := append([]ExportTicket(nil), tickets...)
	for len(pending) > 0 {
		progress := false
		next := make([]ExportTicket, 0, len(pending))
		for _, value := range pending {
			var parentID *string
			if value.ParentSourceID != nil {
				mapped := createdIDs[*value.ParentSourceID]
				if mapped == "" {
					next = append(next, value)
					continue
				}
				parentID = &mapped
			}
			ids := make([]string, 0, len(value.Labels))
			for _, name := range value.Labels {
				ids = append(ids, labelIDs[strings.ToLower(name)])
			}
			dueDate := ""
			if value.DueDate != nil {
				dueDate = value.DueDate.UTC().Format("2006-01-02")
			}
			created, err := s.tickets.CreateTicket(ctx, ticket.CreateTicketRequest{
				Title: value.Title, Description: value.Description, Priority: value.Priority, Type: value.Type,
				ParentID: parentID, DueDate: dueDate, LabelIDs: ids,
			}, projectID, userID)
			if err != nil {
				return nil, err
			}
			createdIDs[value.SourceID] = created.ID
			if value.Status != "open" {
				status := value.Status
				if err := s.tickets.UpdateTicket(ctx, ticket.UpdateTicketRequest{Status: &status, ExpectedVersion: created.Version}, created.ID, userID); err != nil {
					return nil, err
				}
			}
			for _, sourceComment := range value.Comments {
				if _, err := s.comments.CreateComment(ctx, created.ID, userID, sourceComment.Content); err != nil {
					return nil, err
				}
				result.CommentsImported++
			}
			result.TicketsImported++
			progress = true
		}
		if !progress {
			return nil, fmt.Errorf("%w: ticket hierarchy could not be resolved", ErrInvalidBundle)
		}
		pending = next
	}
	return result, nil
}

// ValidateProjectBundle rejects incompatible, oversized, or internally inconsistent imports before writes begin.
func ValidateProjectBundle(bundle ProjectBundle, requireProject bool) error {
	if bundle.Schema != ProjectSchema {
		return fmt.Errorf("%w: schema must be %s", ErrInvalidBundle, ProjectSchema)
	}
	if requireProject && (strings.TrimSpace(bundle.Project.Name) == "" || utf8.RuneCountInString(bundle.Project.Name) > 120 || utf8.RuneCountInString(bundle.Project.Description) > 4000) {
		return fmt.Errorf("%w: invalid project metadata", ErrInvalidBundle)
	}
	if len(bundle.Labels) > maxImportLabels || len(bundle.Tickets) > maxImportTickets {
		return fmt.Errorf("%w: bundle exceeds import limits", ErrInvalidBundle)
	}
	labels := make(map[string]struct{}, len(bundle.Labels))
	for _, value := range bundle.Labels {
		key := strings.ToLower(strings.TrimSpace(value.Name))
		if key == "" || utf8.RuneCountInString(value.Name) > 50 || !portableLabelColor.MatchString(value.Color) {
			return fmt.Errorf("%w: invalid label", ErrInvalidBundle)
		}
		if _, exists := labels[key]; exists {
			return fmt.Errorf("%w: duplicate label %q", ErrInvalidBundle, value.Name)
		}
		labels[key] = struct{}{}
	}
	byID := make(map[string]ExportTicket, len(bundle.Tickets))
	comments := 0
	for _, value := range bundle.Tickets {
		if strings.TrimSpace(value.SourceID) == "" || strings.TrimSpace(value.Title) == "" || utf8.RuneCountInString(value.Title) > 120 || utf8.RuneCountInString(value.Description) > 4000 {
			return fmt.Errorf("%w: invalid ticket metadata", ErrInvalidBundle)
		}
		if !validTicketType(value.Type) || !validTicketStatus(value.Status) || !validTicketPriority(value.Priority) {
			return fmt.Errorf("%w: invalid ticket workflow value", ErrInvalidBundle)
		}
		if _, exists := byID[value.SourceID]; exists {
			return fmt.Errorf("%w: duplicate ticket source_id", ErrInvalidBundle)
		}
		for _, name := range value.Labels {
			if _, exists := labels[strings.ToLower(strings.TrimSpace(name))]; !exists {
				return fmt.Errorf("%w: ticket references unknown label %q", ErrInvalidBundle, name)
			}
		}
		for _, sourceComment := range value.Comments {
			if strings.TrimSpace(sourceComment.Content) == "" || utf8.RuneCountInString(sourceComment.Content) > 20000 {
				return fmt.Errorf("%w: invalid comment content", ErrInvalidBundle)
			}
		}
		comments += len(value.Comments)
		byID[value.SourceID] = value
	}
	if comments > maxImportComments {
		return fmt.Errorf("%w: bundle exceeds comment import limit", ErrInvalidBundle)
	}
	for _, value := range bundle.Tickets {
		if value.ParentSourceID == nil {
			if value.Type == "subtask" {
				return fmt.Errorf("%w: subtask requires a parent", ErrInvalidBundle)
			}
			continue
		}
		parent, exists := byID[*value.ParentSourceID]
		if !exists || ticketTypeRank(parent.Type) <= ticketTypeRank(value.Type) {
			return fmt.Errorf("%w: invalid ticket hierarchy", ErrInvalidBundle)
		}
	}
	return nil
}

func (s *Service) allTickets(ctx context.Context, projectID, userID string) ([]ticket.Ticket, error) {
	return collectPages(500, func(offset int) ([]ticket.Ticket, error) {
		return s.tickets.ListTicketsInProject(ctx, projectID, userID, ticket.TicketListOptions{Limit: 500, Offset: offset})
	})
}

func (s *Service) allMembers(ctx context.Context, projectID, userID string) ([]project.ProjectMember, error) {
	return collectPages(500, func(offset int) ([]project.ProjectMember, error) {
		return s.projects.ListProjectMembers(ctx, projectID, userID, project.ProjectListOptions{Limit: 500, Offset: offset})
	})
}

func (s *Service) allUserProjects(ctx context.Context, userID string) ([]project.Project, error) {
	return collectPages(500, func(offset int) ([]project.Project, error) {
		return s.projects.ListUserProjects(ctx, userID, project.ProjectListOptions{Limit: 500, Offset: offset})
	})
}

func (s *Service) allComments(ctx context.Context, ticketID, userID string) ([]comment.Comment, error) {
	return collectPages(500, func(offset int) ([]comment.Comment, error) {
		return s.comments.ListComments(ctx, ticketID, userID, comment.CommentListOptions{Limit: 500, Offset: offset})
	})
}

func collectPages[T any](pageSize int, fetch func(offset int) ([]T, error)) ([]T, error) {
	var result []T
	for offset := 0; ; offset += pageSize {
		page, err := fetch(offset)
		if err != nil {
			return nil, err
		}
		result = append(result, page...)
		if len(page) < pageSize {
			return result, nil
		}
	}
}

func exportMembers(values []project.ProjectMember) []ExportMember {
	result := make([]ExportMember, 0, len(values))
	for _, value := range values {
		result = append(result, ExportMember{Username: value.Username, Email: value.Email, Handle: value.Handle, Name: value.Name, Role: value.Role, Remote: value.IsRemote})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Handle < result[j].Handle })
	return result
}

func validTicketType(value string) bool {
	return value == "epic" || value == "task" || value == "subtask"
}
func validTicketStatus(value string) bool {
	return value == "open" || value == "in_progress" || value == "review" || value == "done"
}
func validTicketPriority(value string) bool {
	return value == "low" || value == "medium" || value == "high" || value == "urgent"
}
func ticketTypeRank(value string) int {
	switch value {
	case "epic":
		return 3
	case "task":
		return 2
	case "subtask":
		return 1
	default:
		return 0
	}
}
