package remoteinbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
)

const DefaultMaxBodyBytes int64 = 1 << 20

type Verifier interface {
	VerifyRequest(ctx context.Context, req *http.Request, body []byte) (*httpsig.VerifiedRequest, error)
}

type Service struct {
	repo         Repository
	verifier     Verifier
	delivery     DeliveryEnqueuer
	maxBodyBytes int64
}

type DeliveryEnqueuer interface {
	Enqueue(ctx context.Context, activityID string, targetInboxURL string) (*delivery.Delivery, error)
}

type Option func(*Service)

func NewService(repo Repository, verifier Verifier, opts ...Option) *Service {
	service := &Service{
		repo:         repo,
		verifier:     verifier,
		maxBodyBytes: DefaultMaxBodyBytes,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

func WithMaxBodyBytes(size int64) Option {
	return func(s *Service) {
		if size > 0 {
			s.maxBodyBytes = size
		}
	}
}

func WithDelivery(delivery DeliveryEnqueuer) Option {
	return func(s *Service) {
		s.delivery = delivery
	}
}

func (s *Service) MaxBodyBytes() int64 {
	return s.maxBodyBytes
}

func (s *Service) Receive(ctx context.Context, req *http.Request, targetAPID string, body []byte) (*AcceptedActivity, error) {
	targetActorID, err := s.repo.FindLocalActorIDByAPID(ctx, targetAPID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTargetNotFound
		}
		return nil, err
	}

	verified, err := s.verifier.VerifyRequest(ctx, req, body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}

	activity, err := parseActivity(body)
	if err != nil {
		return nil, err
	}
	activity.ActorID = verified.ActorID

	signatureActorAPID := verified.ActorAPID
	if signatureActorAPID == "" {
		signatureActorAPID, err = s.repo.FindActorAPIDByID(ctx, verified.ActorID)
		if err != nil {
			return nil, err
		}
	}
	if activity.ActorAPID != signatureActorAPID {
		return nil, ErrForbiddenActor
	}

	isProjectFollow, err := s.isProjectFollow(ctx, targetActorID, targetAPID, activity)
	if err != nil {
		return nil, err
	}
	isProjectUndoFollow, err := s.isProjectUndoFollow(ctx, targetActorID, targetAPID, activity)
	if err != nil {
		return nil, err
	}
	isProjectCreateNote, err := s.isProjectCreateNote(ctx, targetActorID, targetAPID, activity)
	if err != nil {
		return nil, err
	}
	isProjectCreateTicket, err := s.isProjectCreateTicket(ctx, targetActorID, targetAPID, activity)
	if err != nil {
		return nil, err
	}
	isProjectUpdateTicket, err := s.isProjectUpdateTicket(ctx, targetActorID, targetAPID, activity)
	if err != nil {
		return nil, err
	}
	isProjectAddTicketAssignee, err := s.isProjectAddTicketAssignee(ctx, targetActorID, targetAPID, activity)
	if err != nil {
		return nil, err
	}

	if isProjectCreateNote {
		return s.repo.StoreInboundCreateNote(ctx, targetActorID, activity)
	}
	if isProjectCreateTicket {
		return s.repo.StoreInboundCreateTicket(ctx, targetActorID, activity)
	}
	if isProjectUpdateTicket {
		return s.repo.StoreInboundUpdateTicket(ctx, targetActorID, activity)
	}
	if isProjectAddTicketAssignee {
		return s.repo.StoreInboundAddTicketAssignee(ctx, targetActorID, activity)
	}

	accepted, err := s.repo.StoreInboundActivity(ctx, targetActorID, activity)
	if err != nil {
		return nil, err
	}
	if isProjectFollow && !accepted.Duplicate {
		response, err := s.repo.AcceptProjectFollow(ctx, targetActorID, activity)
		if err != nil {
			return nil, err
		}
		accepted.ResponseActivityID = response.ActivityID
		accepted.ResponseActivityAPID = response.ActivityAPID
		s.enqueueFollowResponse(ctx, response)
	}
	if isProjectUndoFollow && !accepted.Duplicate {
		if err := s.repo.UndoProjectFollow(ctx, targetActorID, activity); err != nil {
			return nil, err
		}
	}
	return accepted, nil
}

func (s *Service) isProjectAddTicketAssignee(ctx context.Context, targetActorID, targetAPID string, activity *InboundActivity) (bool, error) {
	if activity.Type != "Add" {
		return false, nil
	}
	isProjectActor, err := s.repo.IsProjectActor(ctx, targetActorID)
	if err != nil {
		return false, err
	}
	if !isProjectActor {
		return false, nil
	}
	if activity.ObjectAPID == nil || !isAbsoluteURI(*activity.ObjectAPID) {
		return false, fmt.Errorf("%w: add object must be an assignee actor", ErrInvalidActivity)
	}
	if activity.TargetAPID == nil || !isAbsoluteURI(*activity.TargetAPID) {
		return false, fmt.Errorf("%w: add target must be a ticket", ErrInvalidActivity)
	}
	if *activity.TargetAPID == targetAPID {
		return false, fmt.Errorf("%w: add target must be a ticket", ErrInvalidActivity)
	}
	return true, nil
}

func (s *Service) isProjectUpdateTicket(ctx context.Context, targetActorID, targetAPID string, activity *InboundActivity) (bool, error) {
	if activity.Type != "Update" || activity.ObjectTicket == nil {
		return false, nil
	}
	isProjectActor, err := s.repo.IsProjectActor(ctx, targetActorID)
	if err != nil {
		return false, err
	}
	if !isProjectActor {
		return false, nil
	}
	ticket := activity.ObjectTicket
	if err := validateInboundTicketIdentity(ticket, activity.ActorAPID, targetAPID); err != nil {
		return false, err
	}
	if !ticket.HasName && !ticket.HasContent && !ticket.HasStatus && !ticket.HasPriority && !ticket.HasTicketType && !ticket.HasIsResolved {
		return false, fmt.Errorf("%w: ticket update has no projected fields", ErrInvalidActivity)
	}
	if ticket.HasName && strings.TrimSpace(ticket.Name) == "" {
		return false, fmt.Errorf("%w: ticket name", ErrInvalidActivity)
	}
	if ticket.HasStatus {
		if _, ok := normalizeTicketStatus(ticket.Status); !ok {
			return false, fmt.Errorf("%w: ticket status", ErrInvalidActivity)
		}
	}
	if ticket.HasPriority {
		if _, ok := normalizeTicketPriority(ticket.Priority); !ok {
			return false, fmt.Errorf("%w: ticket priority", ErrInvalidActivity)
		}
	}
	if ticket.HasTicketType {
		if _, ok := normalizeTicketType(ticket.TicketType); !ok {
			return false, fmt.Errorf("%w: ticket type", ErrInvalidActivity)
		}
	}
	if ticket.HasStatus && ticket.HasIsResolved {
		status, _ := normalizeTicketStatus(ticket.Status)
		if (status == "done") != ticket.IsResolved {
			return false, fmt.Errorf("%w: ticket status and isResolved conflict", ErrInvalidActivity)
		}
	}
	if activity.TargetAPID != nil && *activity.TargetAPID != targetAPID {
		return false, fmt.Errorf("%w: update target must match inbox actor", ErrInvalidActivity)
	}
	return true, nil
}

func (s *Service) isProjectCreateTicket(ctx context.Context, targetActorID, targetAPID string, activity *InboundActivity) (bool, error) {
	if activity.Type != "Create" || activity.ObjectTicket == nil {
		return false, nil
	}
	isProjectActor, err := s.repo.IsProjectActor(ctx, targetActorID)
	if err != nil {
		return false, err
	}
	if !isProjectActor {
		return false, nil
	}
	ticket := activity.ObjectTicket
	if err := validateInboundTicketIdentity(ticket, activity.ActorAPID, targetAPID); err != nil {
		return false, err
	}
	if strings.TrimSpace(ticket.Name) == "" {
		return false, fmt.Errorf("%w: ticket name", ErrInvalidActivity)
	}
	if _, ok := normalizeTicketPriority(ticket.Priority); !ok || ticket.InvalidFieldType {
		return false, fmt.Errorf("%w: ticket priority", ErrInvalidActivity)
	}
	if _, ok := normalizeTicketType(ticket.TicketType); !ok {
		return false, fmt.Errorf("%w: ticket type", ErrInvalidActivity)
	}
	if activity.TargetAPID != nil && *activity.TargetAPID != targetAPID {
		return false, fmt.Errorf("%w: create target must match inbox actor", ErrInvalidActivity)
	}
	return true, nil
}

func validateInboundTicketIdentity(ticket *InboundTicket, actorAPID, targetAPID string) error {
	if ticket.InvalidFieldType {
		return fmt.Errorf("%w: ticket field type", ErrInvalidActivity)
	}
	if ticket.ID == "" || !isAbsoluteURI(ticket.ID) {
		return fmt.Errorf("%w: ticket id", ErrInvalidActivity)
	}
	if ticket.AttributedTo == "" || ticket.AttributedTo != actorAPID {
		return ErrForbiddenActor
	}
	if ticket.Context != targetAPID {
		return fmt.Errorf("%w: ticket context must match inbox actor", ErrInvalidActivity)
	}
	return nil
}

func (s *Service) isProjectCreateNote(ctx context.Context, targetActorID, targetAPID string, activity *InboundActivity) (bool, error) {
	if activity.Type != "Create" || activity.ObjectNote == nil {
		return false, nil
	}
	isProjectActor, err := s.repo.IsProjectActor(ctx, targetActorID)
	if err != nil {
		return false, err
	}
	if !isProjectActor {
		return false, nil
	}
	note := activity.ObjectNote
	if note.ID == "" || !isAbsoluteURI(note.ID) {
		return false, fmt.Errorf("%w: note id", ErrInvalidActivity)
	}
	if note.AttributedTo == "" || note.AttributedTo != activity.ActorAPID {
		return false, ErrForbiddenActor
	}
	if note.InReplyTo == "" || !isAbsoluteURI(note.InReplyTo) {
		return false, fmt.Errorf("%w: note inReplyTo", ErrInvalidActivity)
	}
	if strings.TrimSpace(note.Content) == "" {
		return false, fmt.Errorf("%w: note content", ErrInvalidActivity)
	}
	if activity.TargetAPID != nil && *activity.TargetAPID != targetAPID {
		return false, fmt.Errorf("%w: create target must match inbox actor", ErrInvalidActivity)
	}
	return true, nil
}

func (s *Service) isProjectFollow(ctx context.Context, targetActorID, targetAPID string, activity *InboundActivity) (bool, error) {
	if activity.Type != "Follow" {
		return false, nil
	}
	if activity.ObjectAPID == nil || *activity.ObjectAPID != targetAPID {
		return false, fmt.Errorf("%w: follow object must match inbox actor", ErrInvalidActivity)
	}
	return s.repo.IsProjectActor(ctx, targetActorID)
}

func (s *Service) isProjectUndoFollow(ctx context.Context, targetActorID, targetAPID string, activity *InboundActivity) (bool, error) {
	if activity.Type != "Undo" {
		return false, nil
	}
	isProjectActor, err := s.repo.IsProjectActor(ctx, targetActorID)
	if err != nil {
		return false, err
	}
	if !isProjectActor {
		return false, nil
	}
	follow := activity.ObjectActivity
	if follow == nil || follow.Type != "Follow" || follow.ID == "" || !isAbsoluteURI(follow.ID) {
		return false, fmt.Errorf("%w: undo object must be an embedded Follow activity", ErrInvalidActivity)
	}
	if follow.ActorAPID != activity.ActorAPID {
		return false, ErrForbiddenActor
	}
	if follow.ObjectAPID != targetAPID {
		return false, fmt.Errorf("%w: undo follow object must match inbox actor", ErrInvalidActivity)
	}
	if activity.TargetAPID != nil && *activity.TargetAPID != targetAPID {
		return false, fmt.Errorf("%w: undo target must match inbox actor", ErrInvalidActivity)
	}
	return true, nil
}

func (s *Service) enqueueFollowResponse(ctx context.Context, response *FollowResponse) {
	if s.delivery == nil || response == nil || response.ActivityID == "" || response.TargetInboxURL == "" {
		return
	}
	if _, err := s.delivery.Enqueue(ctx, response.ActivityID, response.TargetInboxURL); err != nil {
		log.Printf("failed to enqueue ActivityPub Accept{Follow} delivery to %s: %v", response.TargetInboxURL, err)
	}
}

func parseActivity(body []byte) (*InboundActivity, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidActivity, err)
	}

	activityID := extractAPID(raw["id"])
	if activityID == "" || !isAbsoluteURI(activityID) {
		return nil, fmt.Errorf("%w: id", ErrInvalidActivity)
	}

	activityType, err := extractActivityType(raw["type"])
	if err != nil {
		return nil, err
	}

	actorAPID := extractAPID(raw["actor"])
	if actorAPID == "" || !isAbsoluteURI(actorAPID) {
		return nil, fmt.Errorf("%w: actor", ErrInvalidActivity)
	}

	activity := &InboundActivity{
		ID:        activityID,
		Type:      activityType,
		ActorAPID: actorAPID,
		Document:  append([]byte(nil), body...),
	}
	rawObject := raw["object"]
	if objectAPID := extractAPID(rawObject); objectAPID != "" {
		activity.ObjectAPID = &objectAPID
	}
	if objectActivity := extractEmbeddedActivity(rawObject); objectActivity != nil {
		activity.ObjectActivity = objectActivity
		if activity.Type == "Undo" && activity.TargetAPID == nil && objectActivity.ObjectAPID != "" {
			activity.TargetAPID = &objectActivity.ObjectAPID
		}
	}
	if objectNote := extractInboundNote(rawObject); objectNote != nil {
		activity.ObjectNote = objectNote
	}
	if objectTicket := extractInboundTicket(rawObject); objectTicket != nil {
		activity.ObjectTicket = objectTicket
	}
	if targetAPID := extractAPID(raw["target"]); targetAPID != "" {
		activity.TargetAPID = &targetAPID
	}
	return activity, nil
}

func extractActivityType(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		if isSupportedActivityType(typed) {
			return typed, nil
		}
	case []any:
		for _, item := range typed {
			if value, ok := item.(string); ok && isSupportedActivityType(value) {
				return value, nil
			}
		}
	}
	return "", ErrUnsupportedActivity
}

func isSupportedActivityType(value string) bool {
	switch value {
	case "Create", "Update", "Delete", "Add", "Remove", "Invite", "Accept", "Reject", "Follow", "Undo":
		return true
	default:
		return false
	}
}

func extractAPID(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		if id, ok := typed["id"].(string); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

func extractEmbeddedActivity(value any) *EmbeddedActivity {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	activityType, err := extractActivityType(raw["type"])
	if err != nil {
		return nil
	}
	embedded := &EmbeddedActivity{
		ID:         extractAPID(raw["id"]),
		Type:       activityType,
		ActorAPID:  extractAPID(raw["actor"]),
		ObjectAPID: extractAPID(raw["object"]),
	}
	if embedded.ID == "" && embedded.ActorAPID == "" && embedded.ObjectAPID == "" {
		return nil
	}
	return embedded
}

func extractInboundNote(value any) *InboundNote {
	raw, ok := value.(map[string]any)
	if !ok || !hasObjectType(raw["type"], "Note") {
		return nil
	}
	document, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	return &InboundNote{
		ID:           extractAPID(raw["id"]),
		AttributedTo: extractAPID(raw["attributedTo"]),
		InReplyTo:    extractAPID(raw["inReplyTo"]),
		Content:      strings.TrimSpace(stringValue(raw["content"])),
		Document:     document,
	}
}

func extractInboundTicket(value any) *InboundTicket {
	raw, ok := value.(map[string]any)
	if !ok || !hasObjectType(raw["type"], "forge:Ticket") {
		return nil
	}
	document, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	name, hasName, validName := optionalStringValue(raw, "name")
	content, hasContent, validContent := optionalStringValue(raw, "content")
	status, hasStatus, validStatus := optionalStringValue(raw, "forge:status")
	priority, hasPriority, validPriority := optionalStringValue(raw, "forge:priority")
	ticketType, hasTicketType, validTicketType := optionalStringValue(raw, "forge:ticketType")
	isResolved, hasIsResolved, validIsResolved := optionalBoolValue(raw, "forge:isResolved")
	return &InboundTicket{
		ID:               extractAPID(raw["id"]),
		AttributedTo:     extractAPID(raw["attributedTo"]),
		Context:          extractAPID(raw["context"]),
		Name:             strings.TrimSpace(name),
		HasName:          hasName,
		Content:          strings.TrimSpace(content),
		HasContent:       hasContent,
		Status:           strings.TrimSpace(status),
		HasStatus:        hasStatus,
		Priority:         strings.TrimSpace(priority),
		HasPriority:      hasPriority,
		TicketType:       strings.TrimSpace(ticketType),
		HasTicketType:    hasTicketType,
		IsResolved:       isResolved,
		HasIsResolved:    hasIsResolved,
		InvalidFieldType: !validName || !validContent || !validStatus || !validPriority || !validTicketType || !validIsResolved,
		Document:         document,
	}
}

func normalizeTicketStatus(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "open", "in_progress", "review", "done":
		return value, true
	default:
		return "", false
	}
}

func normalizeTicketPriority(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "medium", true
	}
	switch value {
	case "low", "medium", "high", "urgent":
		return value, true
	default:
		return "", false
	}
}

func normalizeTicketType(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "task", true
	}
	switch value {
	case "epic", "task", "subtask":
		return value, true
	default:
		return "", false
	}
}

func hasObjectType(value any, expected string) bool {
	switch typed := value.(type) {
	case string:
		return typed == expected
	case []any:
		for _, item := range typed {
			if value, ok := item.(string); ok && value == expected {
				return true
			}
		}
	}
	return false
}

func stringValue(value any) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func optionalStringValue(raw map[string]any, key string) (string, bool, bool) {
	value, exists := raw[key]
	if !exists {
		return "", false, true
	}
	typed, ok := value.(string)
	return typed, true, ok
}

func optionalBoolValue(raw map[string]any, key string) (bool, bool, bool) {
	value, exists := raw[key]
	if !exists {
		return false, false, true
	}
	typed, ok := value.(bool)
	return typed, true, ok
}

func isAbsoluteURI(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func isActivityMediaType(value string) bool {
	if value == "" {
		return false
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	return mediaType == "application/activity+json" || mediaType == "application/ld+json"
}
