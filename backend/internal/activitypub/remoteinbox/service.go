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
	"github.com/antonovs105/project-management-system-go/internal/activitypub/domainblock"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
)

// DefaultMaxBodyBytes is the default maximum accepted inbox request body size.
const DefaultMaxBodyBytes int64 = 1 << 20

// Verifier validates HTTP signatures for inbound inbox requests.
type Verifier interface {
	VerifyRequest(ctx context.Context, req *http.Request, body []byte) (*httpsig.VerifiedRequest, error)
}

// Service verifies, validates, stores, and fans out remote inbox activities.
type Service struct {
	repo           Repository
	verifier       Verifier
	delivery       DeliveryEnqueuer
	maxBodyBytes   int64
	blockedDomains map[string]struct{}
}

// DeliveryEnqueuer queues response and fan-out deliveries produced by inbound activity handling.
type DeliveryEnqueuer interface {
	Enqueue(ctx context.Context, activityID string, targetInboxURL string) (*delivery.Delivery, error)
	EnqueueWithActor(ctx context.Context, activityID string, actorID string, targetInboxURL string) (*delivery.Delivery, error)
}

// Option configures an inbound inbox service.
type Option func(*Service)

// NewService creates an inbound inbox service.
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

// WithMaxBodyBytes overrides the maximum accepted inbox request body size.
func WithMaxBodyBytes(size int64) Option {
	return func(s *Service) {
		if size > 0 {
			s.maxBodyBytes = size
		}
	}
}

// WithDelivery attaches a delivery queue for follow responses and project fan-out.
func WithDelivery(delivery DeliveryEnqueuer) Option {
	return func(s *Service) {
		s.delivery = delivery
	}
}

// WithBlockedDomains preloads blocked actor domains from configuration.
func WithBlockedDomains(domains []string) Option {
	return func(s *Service) {
		for _, domain := range domains {
			normalized := domainblock.Normalize(domain)
			if normalized == "" {
				continue
			}
			if s.blockedDomains == nil {
				s.blockedDomains = make(map[string]struct{})
			}
			s.blockedDomains[normalized] = struct{}{}
		}
	}
}

// MaxBodyBytes returns the configured maximum accepted inbox request body size.
func (s *Service) MaxBodyBytes() int64 {
	return s.maxBodyBytes
}

// isActorDomainBlocked checks configured and persisted domain blocks for an actor.
func (s *Service) isActorDomainBlocked(ctx context.Context, actorAPID string) (bool, error) {
	domain, err := domainblock.FromActorID(actorAPID)
	if err != nil {
		return false, nil
	}
	if domainblock.Contains(s.blockedDomains, domain) {
		return true, nil
	}
	blocked, err := s.repo.IsDomainBlocked(ctx, domainblock.Candidates(domain))
	if err != nil {
		return false, err
	}
	return blocked, nil
}

// Receive verifies and applies a remote ActivityPub inbox activity.
func (s *Service) Receive(ctx context.Context, req *http.Request, targetAPID string, body []byte) (*AcceptedActivity, error) {
	targetActorID, err := s.repo.FindLocalActorIDByAPID(ctx, targetAPID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTargetNotFound
		}
		return nil, err
	}

	activity, err := parseActivity(body)
	if err != nil {
		return nil, err
	}
	blocked, err := s.isActorDomainBlocked(ctx, activity.ActorAPID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, ErrBlockedDomain
	}

	verified, err := s.verifier.VerifyRequest(ctx, req, body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnauthorized, err)
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
		if !isProjectForwardedActivity(signatureActorAPID, activity) {
			return nil, ErrForbiddenActor
		}
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
	isProjectRemoveTicketAssignee, err := s.isProjectRemoveTicketAssignee(ctx, targetActorID, targetAPID, activity)
	if err != nil {
		return nil, err
	}
	isProjectDeleteTicket, err := s.isProjectDeleteTicket(ctx, targetActorID, targetAPID, activity)
	if err != nil {
		return nil, err
	}
	isProjectAcceptInvite, err := s.isProjectInviteResponse(ctx, targetActorID, targetAPID, activity, "Accept")
	if err != nil {
		return nil, err
	}
	isProjectRejectInvite, err := s.isProjectInviteResponse(ctx, targetActorID, targetAPID, activity, "Reject")
	if err != nil {
		return nil, err
	}

	if isProjectCreateNote {
		accepted, err := s.repo.StoreInboundCreateNote(ctx, targetActorID, activity)
		if err != nil {
			return nil, err
		}
		s.enqueueProjectFanOut(ctx, targetActorID, activity, accepted)
		return accepted, nil
	}
	if isProjectCreateTicket {
		accepted, err := s.repo.StoreInboundCreateTicket(ctx, targetActorID, activity)
		if err != nil {
			return nil, err
		}
		s.enqueueProjectFanOut(ctx, targetActorID, activity, accepted)
		return accepted, nil
	}
	if isProjectUpdateTicket {
		accepted, err := s.repo.StoreInboundUpdateTicket(ctx, targetActorID, activity)
		if err != nil {
			return nil, err
		}
		s.enqueueProjectFanOut(ctx, targetActorID, activity, accepted)
		return accepted, nil
	}
	if isProjectAddTicketAssignee {
		accepted, err := s.repo.StoreInboundAddTicketAssignee(ctx, targetActorID, activity)
		if err != nil {
			return nil, err
		}
		s.enqueueProjectFanOut(ctx, targetActorID, activity, accepted)
		return accepted, nil
	}
	if isProjectRemoveTicketAssignee {
		accepted, err := s.repo.StoreInboundRemoveTicketAssignee(ctx, targetActorID, activity)
		if err != nil {
			return nil, err
		}
		s.enqueueProjectFanOut(ctx, targetActorID, activity, accepted)
		return accepted, nil
	}
	if isProjectDeleteTicket {
		accepted, err := s.repo.StoreInboundDeleteTicket(ctx, targetActorID, activity)
		if err != nil {
			return nil, err
		}
		s.enqueueProjectFanOut(ctx, targetActorID, activity, accepted)
		return accepted, nil
	}
	if isProjectAcceptInvite {
		return s.repo.StoreInboundAcceptInvite(ctx, targetActorID, activity)
	}
	if isProjectRejectInvite {
		return s.repo.StoreInboundRejectInvite(ctx, targetActorID, activity)
	}

	accepted, err := s.repo.StoreInboundActivity(ctx, targetActorID, activity)
	if err != nil {
		return nil, err
	}
	if isProjectFollow && !accepted.Duplicate {
		// A project Follow is a subscription request, not collaboration access.
		// Project write access is granted by Invite/Accept so random remote actors
		// cannot become contributors by following a project actor.
		return accepted, nil
	}
	if isProjectUndoFollow && !accepted.Duplicate {
		if err := s.repo.UndoProjectFollow(ctx, targetActorID, activity); err != nil {
			return nil, err
		}
	}
	return accepted, nil
}

// isProjectInviteResponse validates Accept or Reject responses for project invites.
func (s *Service) isProjectInviteResponse(ctx context.Context, targetActorID, targetAPID string, activity *InboundActivity, activityType string) (bool, error) {
	if activity.Type != activityType {
		return false, nil
	}
	isProjectActor, err := s.repo.IsProjectActor(ctx, targetActorID)
	if err != nil {
		return false, err
	}
	if !isProjectActor {
		return false, nil
	}
	activityName := strings.ToLower(activityType)
	if activity.ObjectAPID == nil || !isAbsoluteURI(*activity.ObjectAPID) {
		return false, fmt.Errorf("%w: %s object must be an invite", ErrInvalidActivity, activityName)
	}
	if *activity.ObjectAPID == targetAPID {
		return false, fmt.Errorf("%w: %s object must be an invite", ErrInvalidActivity, activityName)
	}
	if activity.ObjectActivity != nil && activity.ObjectActivity.Type != "Invite" {
		return false, fmt.Errorf("%w: %s object must be an invite", ErrInvalidActivity, activityName)
	}
	if activity.TargetAPID != nil && *activity.TargetAPID != targetAPID {
		return false, fmt.Errorf("%w: %s target must match inbox actor", ErrInvalidActivity, activityName)
	}
	return true, nil
}

// enqueueProjectFanOut queues accepted project activities to other remote followers.
func (s *Service) enqueueProjectFanOut(ctx context.Context, targetActorID string, activity *InboundActivity, accepted *AcceptedActivity) {
	if s.delivery == nil || accepted == nil || accepted.Duplicate || accepted.ActivityID == "" {
		return
	}
	inboxes, err := s.repo.RemoteProjectFollowerInboxesExceptActor(ctx, targetActorID, activity.ActorID)
	if err != nil {
		log.Printf("failed to load ActivityPub fan-out recipients for project %s activity %s: %v", targetActorID, accepted.ActivityID, err)
		return
	}
	for _, inbox := range inboxes {
		if inbox == "" {
			continue
		}
		if _, err := s.delivery.EnqueueWithActor(ctx, accepted.ActivityID, targetActorID, inbox); err != nil {
			log.Printf("failed to enqueue ActivityPub fan-out for project %s activity %s inbox %s: %v", targetActorID, accepted.ActivityID, inbox, err)
		}
	}
}

// isProjectDeleteTicket validates a remote Delete activity for a project ticket.
func (s *Service) isProjectDeleteTicket(ctx context.Context, targetActorID, targetAPID string, activity *InboundActivity) (bool, error) {
	if activity.Type != "Delete" {
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
		return false, fmt.Errorf("%w: delete object must be a ticket", ErrInvalidActivity)
	}
	if *activity.ObjectAPID == targetAPID {
		return false, fmt.Errorf("%w: delete object must be a ticket", ErrInvalidActivity)
	}
	if activity.TargetAPID != nil && *activity.TargetAPID != targetAPID {
		return false, fmt.Errorf("%w: delete target must match inbox actor", ErrInvalidActivity)
	}
	return true, nil
}

// isProjectRemoveTicketAssignee validates a remote Remove assignee activity.
func (s *Service) isProjectRemoveTicketAssignee(ctx context.Context, targetActorID, targetAPID string, activity *InboundActivity) (bool, error) {
	return s.isProjectTicketAssigneeActivity(ctx, targetActorID, targetAPID, activity, "Remove")
}

// isProjectAddTicketAssignee validates a remote Add assignee activity.
func (s *Service) isProjectAddTicketAssignee(ctx context.Context, targetActorID, targetAPID string, activity *InboundActivity) (bool, error) {
	return s.isProjectTicketAssigneeActivity(ctx, targetActorID, targetAPID, activity, "Add")
}

// isProjectTicketAssigneeActivity validates shared Add and Remove ticket assignee fields.
func (s *Service) isProjectTicketAssigneeActivity(ctx context.Context, targetActorID, targetAPID string, activity *InboundActivity, activityType string) (bool, error) {
	if activity.Type != activityType {
		return false, nil
	}
	activityName := strings.ToLower(activityType)
	isProjectActor, err := s.repo.IsProjectActor(ctx, targetActorID)
	if err != nil {
		return false, err
	}
	if !isProjectActor {
		return false, nil
	}
	if activity.ObjectAPID == nil || !isAbsoluteURI(*activity.ObjectAPID) {
		return false, fmt.Errorf("%w: %s object must be an assignee actor", ErrInvalidActivity, activityName)
	}
	if activity.TargetAPID == nil || !isAbsoluteURI(*activity.TargetAPID) {
		return false, fmt.Errorf("%w: %s target must be a ticket", ErrInvalidActivity, activityName)
	}
	if *activity.TargetAPID == targetAPID {
		return false, fmt.Errorf("%w: %s target must be a ticket", ErrInvalidActivity, activityName)
	}
	return true, nil
}

// isProjectUpdateTicket validates a remote Update activity for a project ticket.
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

// isProjectCreateTicket validates a remote Create Ticket activity for a project inbox.
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

// validateInboundTicketIdentity checks common remote ticket identity and ownership fields.
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

// isProjectForwardedActivity allows a project actor to fan out an activity it targets.
func isProjectForwardedActivity(signatureActorAPID string, activity *InboundActivity) bool {
	if signatureActorAPID == "" || activity == nil {
		return false
	}
	if activity.TargetAPID != nil && *activity.TargetAPID == signatureActorAPID {
		return true
	}
	return activity.ObjectTicket != nil && activity.ObjectTicket.Context == signatureActorAPID
}

// isProjectCreateNote validates a remote Create Note activity for a project ticket.
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

// isProjectFollow validates a remote Follow activity addressed to a project actor.
func (s *Service) isProjectFollow(ctx context.Context, targetActorID, targetAPID string, activity *InboundActivity) (bool, error) {
	if activity.Type != "Follow" {
		return false, nil
	}
	if activity.ObjectAPID == nil || *activity.ObjectAPID != targetAPID {
		return false, fmt.Errorf("%w: follow object must match inbox actor", ErrInvalidActivity)
	}
	return s.repo.IsProjectActor(ctx, targetActorID)
}

// isProjectUndoFollow validates an Undo{Follow} activity for a project actor.
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

// enqueueFollowResponse queues an Accept{Follow} response when delivery is configured.
func (s *Service) enqueueFollowResponse(ctx context.Context, response *FollowResponse) {
	if s.delivery == nil || response == nil || response.ActivityID == "" || response.TargetInboxURL == "" {
		return
	}
	if _, err := s.delivery.Enqueue(ctx, response.ActivityID, response.TargetInboxURL); err != nil {
		log.Printf("failed to enqueue ActivityPub Accept{Follow} delivery to %s: %v", response.TargetInboxURL, err)
	}
}

// parseActivity decodes an inbound ActivityStreams activity into normalized fields.
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

// extractActivityType returns the first supported ActivityStreams type value.
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

// isSupportedActivityType reports whether an inbound activity type is implemented.
func isSupportedActivityType(value string) bool {
	switch value {
	case "Create", "Update", "Delete", "Add", "Remove", "Invite", "Accept", "Reject", "Follow", "Undo":
		return true
	default:
		return false
	}
}

// extractAPID extracts an ActivityPub ID from a string or embedded object.
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

// extractEmbeddedActivity normalizes an embedded activity object such as Undo{Follow}.
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

// extractInboundNote normalizes an embedded Note object for comment projection.
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

// extractInboundTicket normalizes an embedded ForgeFed Ticket object.
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

// normalizeTicketStatus validates and normalizes inbound ticket workflow status.
func normalizeTicketStatus(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "open", "in_progress", "review", "done":
		return value, true
	default:
		return "", false
	}
}

// normalizeTicketPriority validates and normalizes inbound ticket priority.
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

// normalizeTicketType validates and normalizes inbound ticket type.
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

// hasObjectType reports whether an ActivityStreams object type contains expected.
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

// stringValue returns a string value or empty string for non-string JSON fields.
func stringValue(value any) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

// optionalStringValue reads an optional string field and reports type validity.
func optionalStringValue(raw map[string]any, key string) (string, bool, bool) {
	value, exists := raw[key]
	if !exists {
		return "", false, true
	}
	typed, ok := value.(string)
	return typed, true, ok
}

// optionalBoolValue reads an optional boolean field and reports type validity.
func optionalBoolValue(raw map[string]any, key string) (bool, bool, bool) {
	value, exists := raw[key]
	if !exists {
		return false, false, true
	}
	typed, ok := value.(bool)
	return typed, true, ok
}

// isAbsoluteURI reports whether a value is an absolute HTTP-style URI.
func isAbsoluteURI(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

// isActivityMediaType reports whether Content-Type can contain an ActivityPub body.
func isActivityMediaType(value string) bool {
	if value == "" {
		return false
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	return mediaType == "application/activity+json" || mediaType == "application/ld+json"
}
