package notification

import (
	"context"
	"database/sql"
	"testing"
	"time"

	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type serviceRepository struct {
	created     []Notification
	preferences map[string]Preference
	email       string
	mentionIDs  []string
	due         []DueCandidate
	federation  []FederationRecipient
}

func (r *serviceRepository) Create(_ context.Context, value *Notification) error {
	value.CreatedAt = time.Now().UTC()
	r.created = append(r.created, *value)
	return nil
}

func (*serviceRepository) ListByUserID(context.Context, string, ListOptions) ([]Notification, error) {
	return nil, nil
}

func (*serviceRepository) MarkRead(context.Context, string, string) (*Notification, error) {
	return nil, nil
}

func (*serviceRepository) MarkAllRead(context.Context, string) error { return nil }

func (r *serviceRepository) ListPreferences(context.Context, string) ([]Preference, error) {
	values := make([]Preference, 0, len(r.preferences))
	for _, value := range r.preferences {
		values = append(values, value)
	}
	return values, nil
}

func (r *serviceRepository) GetPreference(_ context.Context, _ string, notificationType string) (*Preference, error) {
	value, ok := r.preferences[notificationType]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return &value, nil
}

func (r *serviceRepository) UpsertPreference(_ context.Context, _ string, value Preference) (*Preference, error) {
	r.preferences[value.Type] = value
	return &value, nil
}

func (r *serviceRepository) VerifiedEmail(context.Context, string) (string, error) {
	if r.email == "" {
		return "", sql.ErrNoRows
	}
	return r.email, nil
}

func (r *serviceRepository) ResolveLocalUserIDs(context.Context, []string) ([]string, error) {
	return r.mentionIDs, nil
}

func (r *serviceRepository) ListDueCandidates(context.Context, time.Time) ([]DueCandidate, error) {
	return r.due, nil
}

func (r *serviceRepository) FederationFailureRecipients(context.Context, string) ([]FederationRecipient, error) {
	return r.federation, nil
}

type queuedEmail struct {
	recipient string
	subject   string
	body      string
}

type serviceEmailQueue struct{ values []queuedEmail }

func (q *serviceEmailQueue) QueueEmail(_ context.Context, recipient, subject, body string) error {
	q.values = append(q.values, queuedEmail{recipient: recipient, subject: subject, body: body})
	return nil
}

func TestNotifyTicketAssignedUsesDefaultInAppAndEmailDelivery(t *testing.T) {
	repository := &serviceRepository{preferences: map[string]Preference{}, email: "member@example.test"}
	email := &serviceEmailQueue{}
	service := NewService(repository, WithEmailQueue(email, "https://progo.example.test"))
	assigneeID := uuid.NewString()
	actorID := uuid.NewString()
	projectID := uuid.NewString()
	ticketID := uuid.NewString()

	err := service.NotifyTicketAssigned(context.Background(), assigneeID, actorID, ticket.Ticket{
		ID: ticketID, ProjectID: projectID, Title: "Review evidence",
	})
	require.NoError(t, err)
	require.Len(t, repository.created, 1)
	require.Equal(t, TypeTicketAssigned, repository.created[0].Type)
	require.Len(t, email.values, 1)
	require.Equal(t, "member@example.test", email.values[0].recipient)
	require.Contains(t, email.values[0].body, "/projects/"+projectID)
}

func TestNotifyTicketAssignedHonorsMutedPreference(t *testing.T) {
	repository := &serviceRepository{preferences: map[string]Preference{
		TypeTicketAssigned: {Type: TypeTicketAssigned, InAppEnabled: false, EmailEnabled: false},
	}}
	email := &serviceEmailQueue{}
	service := NewService(repository, WithEmailQueue(email, "https://progo.example.test"))

	err := service.NotifyTicketAssigned(context.Background(), uuid.NewString(), uuid.NewString(), ticket.Ticket{
		ID: uuid.NewString(), ProjectID: uuid.NewString(), Title: "Muted",
	})
	require.NoError(t, err)
	require.Empty(t, repository.created)
	require.Empty(t, email.values)
}

func TestListAndUpdatePreferencesExposeCompleteValidatedCatalog(t *testing.T) {
	userID := uuid.NewString()
	repository := &serviceRepository{preferences: map[string]Preference{
		TypeCommentCreated: {Type: TypeCommentCreated, InAppEnabled: false, EmailEnabled: true},
	}}
	service := NewService(repository)

	values, err := service.ListPreferences(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, values, len(SupportedTypes))
	require.Equal(t, TypeTicketAssigned, values[0].Type)
	require.True(t, values[0].EmailEnabled)

	updated, err := service.UpdatePreference(context.Background(), userID, Preference{
		Type: TypeTicketStatusChanged, InAppEnabled: true, EmailEnabled: true,
	})
	require.NoError(t, err)
	require.True(t, updated.EmailEnabled)
	_, err = service.UpdatePreference(context.Background(), userID, Preference{Type: "unsupported"})
	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestNotifyCommentCreatedSeparatesMentionsAndParticipants(t *testing.T) {
	reporterID := uuid.NewString()
	assigneeID := uuid.NewString()
	mentionedID := uuid.NewString()
	actorID := uuid.NewString()
	repository := &serviceRepository{
		preferences: map[string]Preference{},
		mentionIDs:  []string{mentionedID},
	}
	service := NewService(repository)
	err := service.NotifyCommentCreated(context.Background(), actorID, ticket.Ticket{
		ID: uuid.NewString(), ProjectID: uuid.NewString(), ReporterID: reporterID, AssigneeID: &assigneeID,
	}, "Please check this, @Reviewer.\nThanks.")
	require.NoError(t, err)
	require.Len(t, repository.created, 3)
	typesByUser := make(map[string]string)
	for _, value := range repository.created {
		typesByUser[value.UserID] = value.Type
		require.NotContains(t, value.Body, "\n")
	}
	require.Equal(t, TypeCommentMentioned, typesByUser[mentionedID])
	require.Equal(t, TypeCommentCreated, typesByUser[reporterID])
	require.Equal(t, TypeCommentCreated, typesByUser[assigneeID])
	require.Equal(t, []string{"reviewer"}, mentionedUsernames("@Reviewer and @reviewer"))
}

func TestNotifyTicketStatusChangedDeduplicatesAndExcludesActor(t *testing.T) {
	actorID := uuid.NewString()
	recipientID := uuid.NewString()
	repository := &serviceRepository{preferences: map[string]Preference{}}
	service := NewService(repository)
	err := service.NotifyTicketStatusChanged(context.Background(), []string{actorID, recipientID, recipientID}, actorID, ticket.Ticket{
		ID: uuid.NewString(), ProjectID: uuid.NewString(), Title: "Ship", Status: "done",
	}, "review")
	require.NoError(t, err)
	require.Len(t, repository.created, 1)
	require.Equal(t, recipientID, repository.created[0].UserID)
	require.Contains(t, repository.created[0].Body, "review to done")
}

func TestDispatchDueNotificationsCreatesDeduplicatedReminders(t *testing.T) {
	repository := &serviceRepository{
		preferences: map[string]Preference{},
		due: []DueCandidate{
			{UserID: uuid.NewString(), ProjectID: uuid.NewString(), TicketID: uuid.NewString(), Title: "Soon", Type: TypeTicketDueSoon, DedupeKey: "soon-key"},
			{UserID: uuid.NewString(), ProjectID: uuid.NewString(), TicketID: uuid.NewString(), Title: "Late", Type: TypeTicketOverdue, DedupeKey: "late-key"},
		},
	}
	service := NewService(repository)
	processed, err := service.DispatchDueNotifications(context.Background(), time.Now())
	require.NoError(t, err)
	require.Equal(t, 2, processed)
	require.Len(t, repository.created, 2)
	require.Equal(t, TypeTicketDueSoon, repository.created[0].Type)
	require.Equal(t, "soon-key", *repository.created[0].DedupeKey)
	require.True(t, repository.created[0].InAppVisible)
	require.Equal(t, TypeTicketOverdue, repository.created[1].Type)
}

func TestNotifyFederationDeliveryFailedUsesSafeHostAndDedupeKey(t *testing.T) {
	projectID := uuid.NewString()
	ownerID := uuid.NewString()
	repository := &serviceRepository{
		preferences: map[string]Preference{},
		federation:  []FederationRecipient{{UserID: ownerID, ProjectID: &projectID}},
	}
	service := NewService(repository)
	deliveryID := uuid.NewString()
	err := service.NotifyFederationDeliveryFailed(context.Background(), apdelivery.Delivery{
		ID: deliveryID, ActorID: projectID, TargetInboxURL: "https://remote.example.test/inbox?private=value",
	}, apdelivery.FailureKindNetwork)
	require.NoError(t, err)
	require.Len(t, repository.created, 1)
	require.Equal(t, TypeFederationDeliveryFailed, repository.created[0].Type)
	require.Equal(t, deliveryID, *repository.created[0].DedupeKey)
	require.Contains(t, repository.created[0].Body, "remote.example.test")
	require.NotContains(t, repository.created[0].Body, "private=value")
}
