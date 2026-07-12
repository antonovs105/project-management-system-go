package notification

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/google/uuid"
)

const (
	// defaultListLimit is the default number of notifications returned.
	defaultListLimit = 50
	// maxListLimit caps notification list responses.
	maxListLimit = 100
)

// ErrInvalidInput reports malformed notification API input.
var ErrInvalidInput = errors.New("invalid notification input")

// ErrNotFound reports missing or inaccessible notifications.
var ErrNotFound = errors.New("notification not found")

// mentionPattern recognizes bounded local @username references in comment text.
var mentionPattern = regexp.MustCompile(`(?i)(?:^|[^[:alnum:]_])@([a-z0-9_.-]{1,64})`)

// Service coordinates notification persistence and realtime fanout.
type Service struct {
	repo      Repository
	events    EventPublisher
	email     EmailQueue
	publicURL string
}

// EmailQueue durably queues a plain-text notification email.
type EmailQueue interface {
	QueueEmail(ctx context.Context, recipient, subject, body string) error
}

// Option customizes the notification service.
type Option func(*Service)

// WithEventPublisher attaches realtime notification fanout.
func WithEventPublisher(events EventPublisher) Option {
	return func(s *Service) {
		s.events = events
	}
}

// WithEmailQueue enables preference-controlled durable email delivery.
func WithEmailQueue(email EmailQueue, publicURL string) Option {
	return func(s *Service) {
		s.email = email
		s.publicURL = strings.TrimRight(strings.TrimSpace(publicURL), "/")
	}
}

// NewService creates a notification service.
func NewService(repo Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

// NotifyTicketAssigned creates a local notification for a ticket assignment.
func (s *Service) NotifyTicketAssigned(ctx context.Context, assigneeID, actorID string, item ticket.Ticket) error {
	if _, err := uuid.Parse(assigneeID); err != nil {
		return ErrInvalidInput
	}
	if _, err := uuid.Parse(actorID); err != nil {
		return ErrInvalidInput
	}
	notification := &Notification{
		UserID:    assigneeID,
		ActorID:   &actorID,
		ProjectID: &item.ProjectID,
		TicketID:  &item.ID,
		Type:      TypeTicketAssigned,
		Title:     "Ticket assigned",
		Body:      fmt.Sprintf("%s was assigned to you.", item.Title),
	}
	return s.deliver(ctx, notification)
}

// NotifyTicketStatusChanged alerts distinct local ticket participants.
func (s *Service) NotifyTicketStatusChanged(ctx context.Context, recipientIDs []string, actorID string, item ticket.Ticket, previousStatus string) error {
	return s.notifyRecipients(ctx, recipientIDs, actorID, func(userID string) *Notification {
		return &Notification{
			UserID: userID, ActorID: &actorID, ProjectID: &item.ProjectID, TicketID: &item.ID,
			Type: TypeTicketStatusChanged, Title: "Ticket status changed",
			Body: fmt.Sprintf("%s moved from %s to %s.", item.Title, previousStatus, item.Status),
		}
	})
}

// NotifyCommentCreated alerts mentioned users and other ticket participants.
func (s *Service) NotifyCommentCreated(ctx context.Context, actorID string, item ticket.Ticket, content string) error {
	usernames := mentionedUsernames(content)
	mentionedIDs, err := s.repo.ResolveLocalUserIDs(ctx, usernames)
	if err != nil {
		return err
	}
	mentioned := make(map[string]struct{}, len(mentionedIDs))
	for _, userID := range mentionedIDs {
		mentioned[userID] = struct{}{}
	}
	var failures []error
	if err := s.notifyRecipients(ctx, mentionedIDs, actorID, func(userID string) *Notification {
		return &Notification{
			UserID: userID, ActorID: &actorID, ProjectID: &item.ProjectID, TicketID: &item.ID,
			Type: TypeCommentMentioned, Title: "You were mentioned",
			Body: shortNotificationBody(content),
		}
	}); err != nil {
		failures = append(failures, err)
	}
	participants := []string{item.ReporterID}
	if item.AssigneeID != nil {
		participants = append(participants, *item.AssigneeID)
	}
	if err := s.notifyRecipients(ctx, participants, actorID, func(userID string) *Notification {
		if _, ok := mentioned[userID]; ok {
			return nil
		}
		return &Notification{
			UserID: userID, ActorID: &actorID, ProjectID: &item.ProjectID, TicketID: &item.ID,
			Type: TypeCommentCreated, Title: "New ticket comment",
			Body: shortNotificationBody(content),
		}
	}); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

// NotifyProjectInvited alerts a local project invitee.
func (s *Service) NotifyProjectInvited(ctx context.Context, inviteeID, actorID, projectID string) error {
	return s.deliver(ctx, &Notification{
		UserID: inviteeID, ActorID: &actorID, ProjectID: &projectID,
		Type: TypeProjectInvited, Title: "Project invitation", Body: "You were invited to join a project.",
	})
}

// NotifyProjectRoleChanged alerts a local member about an access change.
func (s *Service) NotifyProjectRoleChanged(ctx context.Context, userID, actorID, projectID, roleName string) error {
	return s.deliver(ctx, &Notification{
		UserID: userID, ActorID: &actorID, ProjectID: &projectID,
		Type: TypeProjectRoleChanged, Title: "Project role changed",
		Body: fmt.Sprintf("Your project role is now %s.", strings.TrimSpace(roleName)),
	})
}

// NotifyFederationDeliveryFailed alerts a local actor or project owner once.
func (s *Service) NotifyFederationDeliveryFailed(ctx context.Context, value apdelivery.Delivery, failureKind string) error {
	recipients, err := s.repo.FederationFailureRecipients(ctx, value.ActorID)
	if err != nil {
		return err
	}
	target := "remote inbox"
	if parsed, parseErr := url.Parse(value.TargetInboxURL); parseErr == nil && parsed.Hostname() != "" {
		target = parsed.Hostname()
	}
	var failures []error
	for _, recipient := range recipients {
		deliveryID := value.ID
		actorID := value.ActorID
		if err := s.deliver(ctx, &Notification{
			UserID: recipient.UserID, ActorID: &actorID, ProjectID: recipient.ProjectID,
			Type: TypeFederationDeliveryFailed, Title: "Federation delivery failed",
			Body:      fmt.Sprintf("Delivery to %s failed permanently (%s).", target, strings.TrimSpace(failureKind)),
			DedupeKey: &deliveryID,
		}); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// NotifySecurityEvent delivers one preference-controlled account security alert.
func (s *Service) NotifySecurityEvent(ctx context.Context, userID, subject, body string) error {
	return s.deliver(ctx, &Notification{
		UserID: userID, Type: TypeSecurityEvent,
		Title: strings.TrimSpace(subject), Body: strings.TrimSpace(body),
	})
}

// DispatchDueNotifications emits deduplicated due-soon and overdue reminders.
func (s *Service) DispatchDueNotifications(ctx context.Context, now time.Time) (int, error) {
	values, err := s.repo.ListDueCandidates(ctx, now.UTC())
	if err != nil {
		return 0, err
	}
	processed := 0
	var failures []error
	for _, candidate := range values {
		projectID := candidate.ProjectID
		ticketID := candidate.TicketID
		dedupeKey := candidate.DedupeKey
		title := "Ticket due soon"
		body := candidate.Title + " is due within 24 hours."
		if candidate.Type == TypeTicketOverdue {
			title = "Ticket overdue"
			body = candidate.Title + " is overdue."
		}
		err := s.deliver(ctx, &Notification{
			UserID: candidate.UserID, ProjectID: &projectID, TicketID: &ticketID,
			Type: candidate.Type, Title: title, Body: body, DedupeKey: &dedupeKey,
		})
		if err != nil {
			failures = append(failures, err)
			continue
		}
		processed++
	}
	return processed, errors.Join(failures...)
}

// StartDueNotificationLoop dispatches due reminders until stopped.
func (s *Service) StartDueNotificationLoop(ctx context.Context, interval time.Duration, report func(int, error)) context.CancelFunc {
	loopContext, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			processed, err := s.DispatchDueNotifications(loopContext, time.Now().UTC())
			if report != nil {
				report(processed, err)
			}
			select {
			case <-loopContext.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return cancel
}

// ListPreferences returns a complete preference catalog with defaults filled in.
func (s *Service) ListPreferences(ctx context.Context, userID string) ([]Preference, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, ErrInvalidInput
	}
	overrides, err := s.repo.ListPreferences(ctx, userID)
	if err != nil {
		return nil, err
	}
	byType := make(map[string]Preference, len(overrides))
	for _, value := range overrides {
		byType[value.Type] = value
	}
	values := make([]Preference, 0, len(SupportedTypes))
	for _, notificationType := range SupportedTypes {
		if value, ok := byType[notificationType]; ok {
			values = append(values, value)
			continue
		}
		values = append(values, defaultPreference(notificationType))
	}
	return values, nil
}

// UpdatePreference stores one validated delivery preference.
func (s *Service) UpdatePreference(ctx context.Context, userID string, value Preference) (*Preference, error) {
	if _, err := uuid.Parse(userID); err != nil || !supportedType(value.Type) {
		return nil, ErrInvalidInput
	}
	return s.repo.UpsertPreference(ctx, userID, value)
}

// deliver applies preferences, persists the in-app event, and durably queues email.
func (s *Service) deliver(ctx context.Context, value *Notification) error {
	if value == nil || !supportedType(value.Type) {
		return ErrInvalidInput
	}
	if _, err := uuid.Parse(value.UserID); err != nil {
		return ErrInvalidInput
	}
	preference, err := s.preference(ctx, value.UserID, value.Type)
	if err != nil {
		return err
	}
	if !preference.InAppEnabled && !preference.EmailEnabled {
		return nil
	}
	value.InAppVisible = preference.InAppEnabled
	if err := s.repo.Create(ctx, value); err != nil {
		if errors.Is(err, ErrRecipientNotLocal) || errors.Is(err, ErrDuplicate) {
			return nil
		}
		return err
	}
	if value.InAppVisible && s.events != nil {
		s.events.PublishNotification(*value)
	}
	if !preference.EmailEnabled || s.email == nil {
		return nil
	}
	recipient, err := s.repo.VerifiedEmail(ctx, value.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.email.QueueEmail(ctx, recipient, value.Title, s.emailBody(value))
}

// notifyRecipients deduplicates local recipients and excludes the initiating actor.
func (s *Service) notifyRecipients(ctx context.Context, recipientIDs []string, actorID string, build func(string) *Notification) error {
	seen := make(map[string]struct{}, len(recipientIDs))
	var failures []error
	for _, userID := range recipientIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" || userID == actorID {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		value := build(userID)
		if value == nil {
			continue
		}
		if err := s.deliver(ctx, value); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// mentionedUsernames extracts distinct @username references from comment text.
func mentionedUsernames(content string) []string {
	seen := make(map[string]struct{})
	values := make([]string, 0)
	for _, match := range mentionPattern.FindAllStringSubmatch(content, -1) {
		username := strings.ToLower(match[1])
		if _, ok := seen[username]; ok {
			continue
		}
		seen[username] = struct{}{}
		values = append(values, username)
	}
	return values
}

// shortNotificationBody collapses whitespace and bounds inbox/email previews.
func shortNotificationBody(content string) string {
	content = strings.Join(strings.Fields(content), " ")
	const limit = 240
	if len([]rune(content)) <= limit {
		return content
	}
	return string([]rune(content)[:limit-1]) + "…"
}

// preference loads an override or returns the product default.
func (s *Service) preference(ctx context.Context, userID, notificationType string) (Preference, error) {
	value, err := s.repo.GetPreference(ctx, userID, notificationType)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultPreference(notificationType), nil
	}
	if err != nil {
		return Preference{}, err
	}
	return *value, nil
}

// emailBody renders a plain-text notification and safe first-party link.
func (s *Service) emailBody(value *Notification) string {
	body := strings.TrimSpace(value.Body)
	if s.publicURL == "" {
		return body
	}
	link := s.publicURL
	if value.ProjectID != nil && *value.ProjectID != "" {
		link += "/projects/" + *value.ProjectID
	}
	return body + "\n\nOpen Progo: " + link
}

// defaultPreference enables in-app delivery and email for high-signal events.
func defaultPreference(notificationType string) Preference {
	emailEnabled := false
	switch notificationType {
	case TypeTicketAssigned, TypeTicketDueSoon, TypeTicketOverdue, TypeCommentMentioned, TypeProjectInvited, TypeFederationDeliveryFailed, TypeSecurityEvent:
		emailEnabled = true
	}
	return Preference{Type: notificationType, InAppEnabled: true, EmailEnabled: emailEnabled}
}

// supportedType reports whether a preference type is part of the public catalog.
func supportedType(value string) bool {
	for _, candidate := range SupportedTypes {
		if value == candidate {
			return true
		}
	}
	return false
}

// List returns notifications for a user.
func (s *Service) List(ctx context.Context, userID string, options ListOptions) ([]Notification, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, ErrInvalidInput
	}
	options.Limit = normalizeLimit(options.Limit)
	if options.Offset < 0 {
		options.Offset = 0
	}
	return s.repo.ListByUserID(ctx, userID, options)
}

// MarkRead marks one notification read.
func (s *Service) MarkRead(ctx context.Context, userID, notificationID string) (*Notification, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, ErrInvalidInput
	}
	if _, err := uuid.Parse(notificationID); err != nil {
		return nil, ErrInvalidInput
	}
	notification, err := s.repo.MarkRead(ctx, userID, notificationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return notification, err
}

// MarkAllRead marks all user notifications read.
func (s *Service) MarkAllRead(ctx context.Context, userID string) error {
	if _, err := uuid.Parse(userID); err != nil {
		return ErrInvalidInput
	}
	return s.repo.MarkAllRead(ctx, userID)
}

// normalizeLimit bounds notification list sizes.
func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}
	if limit > maxListLimit {
		return maxListLimit
	}
	return limit
}
