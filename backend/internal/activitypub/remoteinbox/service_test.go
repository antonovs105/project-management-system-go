package remoteinbox

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryRepository struct {
	targetActorID     string
	actorAPID         string
	stored            *InboundActivity
	storedNote        *InboundActivity
	storedTicket      *InboundActivity
	updatedTicket     *InboundActivity
	assignedTicket    *InboundActivity
	unassignedTicket  *InboundActivity
	deletedTicket     *InboundActivity
	acceptedInvite    *InboundActivity
	rejectedInvite    *InboundActivity
	storeResult       *AcceptedActivity
	projectActor      bool
	fanoutProjectID   string
	fanoutActorID     string
	fanoutInboxes     []string
	fanoutErr         error
	follow            *InboundActivity
	followResult      *FollowResponse
	undo              *InboundActivity
	findErr           error
	storeErr          error
	storeNoteErr      error
	storeTicketErr    error
	updateTicketErr   error
	assignTicketErr   error
	unassignTicketErr error
	deleteTicketErr   error
	acceptInviteErr   error
	rejectInviteErr   error
	followErr         error
	undoErr           error
}

func (m *memoryRepository) FindLocalActorIDByAPID(ctx context.Context, apID string) (string, error) {
	if m.findErr != nil {
		return "", m.findErr
	}
	return m.targetActorID, nil
}

func (m *memoryRepository) FindActorAPIDByID(ctx context.Context, actorID string) (string, error) {
	return m.actorAPID, nil
}

func (m *memoryRepository) IsProjectActor(ctx context.Context, actorID string) (bool, error) {
	return m.projectActor, nil
}

func (m *memoryRepository) RemoteProjectFollowerInboxesExceptActor(ctx context.Context, projectID string, actorID string) ([]string, error) {
	if m.fanoutErr != nil {
		return nil, m.fanoutErr
	}
	m.fanoutProjectID = projectID
	m.fanoutActorID = actorID
	return m.fanoutInboxes, nil
}

func (m *memoryRepository) StoreInboundActivity(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if m.storeErr != nil {
		return nil, m.storeErr
	}
	copy := *activity
	m.stored = &copy
	if m.storeResult != nil {
		return m.storeResult, nil
	}
	return &AcceptedActivity{
		ActivityID:   "stored-activity",
		ActivityAPID: activity.ID,
		ReceivedAt:   time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (m *memoryRepository) StoreInboundCreateNote(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if m.storeNoteErr != nil {
		return nil, m.storeNoteErr
	}
	copy := *activity
	m.storedNote = &copy
	if m.storeResult != nil {
		return m.storeResult, nil
	}
	return &AcceptedActivity{
		ActivityID:   "stored-note-activity",
		ActivityAPID: activity.ID,
		ReceivedAt:   time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (m *memoryRepository) StoreInboundCreateTicket(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if m.storeTicketErr != nil {
		return nil, m.storeTicketErr
	}
	copy := *activity
	m.storedTicket = &copy
	if m.storeResult != nil {
		return m.storeResult, nil
	}
	return &AcceptedActivity{
		ActivityID:   "stored-ticket-activity",
		ActivityAPID: activity.ID,
		ReceivedAt:   time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (m *memoryRepository) StoreInboundUpdateTicket(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if m.updateTicketErr != nil {
		return nil, m.updateTicketErr
	}
	copy := *activity
	m.updatedTicket = &copy
	if m.storeResult != nil {
		return m.storeResult, nil
	}
	return &AcceptedActivity{
		ActivityID:   "updated-ticket-activity",
		ActivityAPID: activity.ID,
		ReceivedAt:   time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (m *memoryRepository) StoreInboundAddTicketAssignee(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if m.assignTicketErr != nil {
		return nil, m.assignTicketErr
	}
	copy := *activity
	m.assignedTicket = &copy
	if m.storeResult != nil {
		return m.storeResult, nil
	}
	return &AcceptedActivity{
		ActivityID:   "assigned-ticket-activity",
		ActivityAPID: activity.ID,
		ReceivedAt:   time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (m *memoryRepository) StoreInboundRemoveTicketAssignee(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if m.unassignTicketErr != nil {
		return nil, m.unassignTicketErr
	}
	copy := *activity
	m.unassignedTicket = &copy
	if m.storeResult != nil {
		return m.storeResult, nil
	}
	return &AcceptedActivity{
		ActivityID:   "unassigned-ticket-activity",
		ActivityAPID: activity.ID,
		ReceivedAt:   time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (m *memoryRepository) StoreInboundDeleteTicket(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if m.deleteTicketErr != nil {
		return nil, m.deleteTicketErr
	}
	copy := *activity
	m.deletedTicket = &copy
	if m.storeResult != nil {
		return m.storeResult, nil
	}
	return &AcceptedActivity{
		ActivityID:   "deleted-ticket-activity",
		ActivityAPID: activity.ID,
		ReceivedAt:   time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (m *memoryRepository) StoreInboundAcceptInvite(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if m.acceptInviteErr != nil {
		return nil, m.acceptInviteErr
	}
	copy := *activity
	m.acceptedInvite = &copy
	if m.storeResult != nil {
		return m.storeResult, nil
	}
	return &AcceptedActivity{
		ActivityID:   "accepted-invite-activity",
		ActivityAPID: activity.ID,
		ReceivedAt:   time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (m *memoryRepository) StoreInboundRejectInvite(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if m.rejectInviteErr != nil {
		return nil, m.rejectInviteErr
	}
	copy := *activity
	m.rejectedInvite = &copy
	if m.storeResult != nil {
		return m.storeResult, nil
	}
	return &AcceptedActivity{
		ActivityID:   "rejected-invite-activity",
		ActivityAPID: activity.ID,
		ReceivedAt:   time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (m *memoryRepository) AcceptProjectFollow(ctx context.Context, targetActorID string, activity *InboundActivity) (*FollowResponse, error) {
	if m.followErr != nil {
		return nil, m.followErr
	}
	copy := *activity
	m.follow = &copy
	if m.followResult != nil {
		return m.followResult, nil
	}
	return &FollowResponse{
		ActivityID:     "accept-activity",
		ActivityAPID:   "http://localhost:8080/activities/accept-activity",
		TargetInboxURL: "https://remote.example/users/alice/inbox",
	}, nil
}

func (m *memoryRepository) UndoProjectFollow(ctx context.Context, targetActorID string, activity *InboundActivity) error {
	if m.undoErr != nil {
		return m.undoErr
	}
	copy := *activity
	m.undo = &copy
	return nil
}

type fakeVerifier struct {
	verified *httpsig.VerifiedRequest
	err      error
}

func (f fakeVerifier) VerifyRequest(ctx context.Context, req *http.Request, body []byte) (*httpsig.VerifiedRequest, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.verified, nil
}

type fakeDelivery struct {
	activityID     string
	actorID        string
	targetInboxURL string
	enqueued       []fakeEnqueuedDelivery
	err            error
}

type fakeEnqueuedDelivery struct {
	ActivityID     string
	ActorID        string
	TargetInboxURL string
}

func (f *fakeDelivery) Enqueue(ctx context.Context, activityID string, targetInboxURL string) (*delivery.Delivery, error) {
	f.activityID = activityID
	f.targetInboxURL = targetInboxURL
	f.enqueued = append(f.enqueued, fakeEnqueuedDelivery{
		ActivityID:     activityID,
		TargetInboxURL: targetInboxURL,
	})
	if f.err != nil {
		return nil, f.err
	}
	return &delivery.Delivery{ID: "delivery-1"}, nil
}

func (f *fakeDelivery) EnqueueWithActor(ctx context.Context, activityID string, actorID string, targetInboxURL string) (*delivery.Delivery, error) {
	f.activityID = activityID
	f.actorID = actorID
	f.targetInboxURL = targetInboxURL
	f.enqueued = append(f.enqueued, fakeEnqueuedDelivery{
		ActivityID:     activityID,
		ActorID:        actorID,
		TargetInboxURL: targetInboxURL,
	})
	if f.err != nil {
		return nil, f.err
	}
	return &delivery.Delivery{ID: "delivery-1"}, nil
}

func TestServiceReceiveStoresValidSignedActivity(t *testing.T) {
	repo := &memoryRepository{targetActorID: "target-actor"}
	verifier := fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
		KeyID:     "https://remote.example/users/alice#main-key",
	}}
	service := NewService(repo, verifier)
	req := newInboxRequest(t, `{"id":"https://remote.example/activities/1","type":"Create","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/notes/1"},"target":"http://localhost:8080/users/bob"}`)

	accepted, err := service.Receive(context.Background(), req, "http://localhost:8080/users/bob", []byte(`{"id":"https://remote.example/activities/1","type":"Create","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/notes/1"},"target":"http://localhost:8080/users/bob"}`))

	require.NoError(t, err)
	assert.Equal(t, "https://remote.example/activities/1", accepted.ActivityAPID)
	require.NotNil(t, repo.stored)
	assert.Equal(t, "remote-actor", repo.stored.ActorID)
	assert.Equal(t, "Create", repo.stored.Type)
	require.NotNil(t, repo.stored.ObjectAPID)
	assert.Equal(t, "https://remote.example/notes/1", *repo.stored.ObjectAPID)
	require.NotNil(t, repo.stored.TargetAPID)
	assert.Equal(t, "http://localhost:8080/users/bob", *repo.stored.TargetAPID)
}

func TestServiceReceiveStoresProjectFollowWithoutGrantingAccess(t *testing.T) {
	repo := &memoryRepository{
		targetActorID: "project-actor",
		projectActor:  true,
	}
	delivery := &fakeDelivery{}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}}, WithDelivery(delivery))
	body := []byte(`{"id":"https://remote.example/activities/follow-1","type":"Follow","actor":"https://remote.example/users/alice","object":"http://localhost:8080/projects/project-1"}`)

	accepted, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.NoError(t, err)
	require.NotNil(t, repo.stored)
	assert.Equal(t, "Follow", repo.stored.Type)
	assert.Empty(t, accepted.ResponseActivityID)
	assert.Nil(t, repo.follow)
	assert.Empty(t, delivery.activityID)
	assert.Empty(t, delivery.targetInboxURL)
}

func TestServiceReceiveDuplicateProjectFollowDoesNotQueueResponse(t *testing.T) {
	repo := &memoryRepository{
		targetActorID: "project-actor",
		projectActor:  true,
		storeResult: &AcceptedActivity{
			ActivityID:   "stored-follow",
			ActivityAPID: "https://remote.example/activities/follow-1",
			ReceivedAt:   time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
			Duplicate:    true,
		},
	}
	delivery := &fakeDelivery{}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}}, WithDelivery(delivery))
	body := []byte(`{"id":"https://remote.example/activities/follow-1","type":"Follow","actor":"https://remote.example/users/alice","object":"http://localhost:8080/projects/project-1"}`)

	accepted, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.NoError(t, err)
	assert.True(t, accepted.Duplicate)
	assert.Nil(t, repo.follow)
	assert.Empty(t, delivery.activityID)
	assert.Empty(t, delivery.targetInboxURL)
}

func TestServiceReceiveRejectsProjectFollowWithWrongObject(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/follow-1","type":"Follow","actor":"https://remote.example/users/alice","object":"http://localhost:8080/projects/other"}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrInvalidActivity)
}

func TestServiceReceiveStoresProjectCreateNote(t *testing.T) {
	repo := &memoryRepository{targetActorID: "project-actor", projectActor: true}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/create-note-1","type":"Create","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/notes/1","type":"Note","attributedTo":"https://remote.example/users/alice","inReplyTo":"http://localhost:8080/tickets/ticket-1","content":"Looks ready."}}`)

	accepted, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.NoError(t, err)
	assert.Equal(t, "https://remote.example/activities/create-note-1", accepted.ActivityAPID)
	require.NotNil(t, repo.storedNote)
	require.NotNil(t, repo.storedNote.ObjectNote)
	assert.Equal(t, "https://remote.example/notes/1", repo.storedNote.ObjectNote.ID)
	assert.Equal(t, "http://localhost:8080/tickets/ticket-1", repo.storedNote.ObjectNote.InReplyTo)
	assert.Equal(t, "Looks ready.", repo.storedNote.ObjectNote.Content)
	require.NotNil(t, repo.storedNote.ObjectAPID)
	assert.Equal(t, "https://remote.example/notes/1", *repo.storedNote.ObjectAPID)
}

func TestServiceReceiveRejectsProjectCreateNoteActorMismatch(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/create-note-1","type":"Create","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/notes/1","type":"Note","attributedTo":"https://remote.example/users/bob","inReplyTo":"http://localhost:8080/tickets/ticket-1","content":"Looks ready."}}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrForbiddenActor)
}

func TestServiceReceiveRejectsProjectCreateNoteWithoutReplyTarget(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/create-note-1","type":"Create","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/notes/1","type":"Note","attributedTo":"https://remote.example/users/alice","content":"Looks ready."}}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrInvalidActivity)
}

func TestServiceReceiveStoresProjectCreateTicket(t *testing.T) {
	repo := &memoryRepository{targetActorID: "project-actor", projectActor: true}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/create-ticket-1","type":"Create","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/tickets/1","type":["forge:Ticket"],"attributedTo":"https://remote.example/users/alice","context":"http://localhost:8080/projects/project-1","name":"Remote ticket","content":"From another server","forge:priority":"high","forge:ticketType":"task","forge:isResolved":false}}`)

	accepted, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.NoError(t, err)
	assert.Equal(t, "https://remote.example/activities/create-ticket-1", accepted.ActivityAPID)
	require.NotNil(t, repo.storedTicket)
	require.NotNil(t, repo.storedTicket.ObjectTicket)
	assert.Equal(t, "https://remote.example/tickets/1", repo.storedTicket.ObjectTicket.ID)
	assert.Equal(t, "http://localhost:8080/projects/project-1", repo.storedTicket.ObjectTicket.Context)
	assert.Equal(t, "Remote ticket", repo.storedTicket.ObjectTicket.Name)
	assert.Equal(t, "high", repo.storedTicket.ObjectTicket.Priority)
	require.NotNil(t, repo.storedTicket.ObjectAPID)
	assert.Equal(t, "https://remote.example/tickets/1", *repo.storedTicket.ObjectAPID)
}

func TestServiceReceiveFansOutProjectCreateTicket(t *testing.T) {
	repo := &memoryRepository{
		targetActorID: "project-actor",
		projectActor:  true,
		fanoutInboxes: []string{
			"https://remote.example/users/bob/inbox",
			"https://remote.example/users/carol/inbox",
		},
	}
	delivery := &fakeDelivery{}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}}, WithDelivery(delivery))
	body := []byte(`{"id":"https://remote.example/activities/create-ticket-1","type":"Create","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/tickets/1","type":["forge:Ticket"],"attributedTo":"https://remote.example/users/alice","context":"http://localhost:8080/projects/project-1","name":"Remote ticket","content":"From another server","forge:priority":"high","forge:ticketType":"task","forge:isResolved":false}}`)

	accepted, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.NoError(t, err)
	assert.Equal(t, "stored-ticket-activity", accepted.ActivityID)
	assert.Equal(t, "project-actor", repo.fanoutProjectID)
	assert.Equal(t, "remote-actor", repo.fanoutActorID)
	require.Len(t, delivery.enqueued, 2)
	assert.Equal(t, "stored-ticket-activity", delivery.enqueued[0].ActivityID)
	assert.Equal(t, "project-actor", delivery.enqueued[0].ActorID)
	assert.Equal(t, "https://remote.example/users/bob/inbox", delivery.enqueued[0].TargetInboxURL)
	assert.Equal(t, "https://remote.example/users/carol/inbox", delivery.enqueued[1].TargetInboxURL)
}

func TestServiceReceiveDuplicateProjectCreateTicketDoesNotFanOut(t *testing.T) {
	repo := &memoryRepository{
		targetActorID: "project-actor",
		projectActor:  true,
		fanoutInboxes: []string{"https://remote.example/users/bob/inbox"},
		storeResult: &AcceptedActivity{
			ActivityID:   "stored-ticket-activity",
			ActivityAPID: "https://remote.example/activities/create-ticket-1",
			ReceivedAt:   time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
			Duplicate:    true,
		},
	}
	delivery := &fakeDelivery{}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}}, WithDelivery(delivery))
	body := []byte(`{"id":"https://remote.example/activities/create-ticket-1","type":"Create","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/tickets/1","type":["forge:Ticket"],"attributedTo":"https://remote.example/users/alice","context":"http://localhost:8080/projects/project-1","name":"Remote ticket","content":"From another server","forge:priority":"high","forge:ticketType":"task","forge:isResolved":false}}`)

	accepted, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.NoError(t, err)
	assert.True(t, accepted.Duplicate)
	assert.Empty(t, repo.fanoutProjectID)
	assert.Empty(t, repo.fanoutActorID)
	assert.Empty(t, delivery.enqueued)
}

func TestServiceReceiveRejectsProjectCreateTicketWrongContext(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/create-ticket-1","type":"Create","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/tickets/1","type":"forge:Ticket","attributedTo":"https://remote.example/users/alice","context":"http://localhost:8080/projects/other","name":"Remote ticket"}}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrInvalidActivity)
}

func TestServiceReceiveRejectsProjectCreateTicketInvalidPriority(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/create-ticket-1","type":"Create","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/tickets/1","type":"forge:Ticket","attributedTo":"https://remote.example/users/alice","context":"http://localhost:8080/projects/project-1","name":"Remote ticket","forge:priority":"eventually"}}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrInvalidActivity)
}

func TestServiceReceiveStoresProjectUpdateTicket(t *testing.T) {
	repo := &memoryRepository{targetActorID: "project-actor", projectActor: true}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/update-ticket-1","type":"Update","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/tickets/1","type":"forge:Ticket","attributedTo":"https://remote.example/users/alice","context":"http://localhost:8080/projects/project-1","name":"Updated remote ticket","forge:status":"done","forge:isResolved":true}}`)

	accepted, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.NoError(t, err)
	assert.Equal(t, "https://remote.example/activities/update-ticket-1", accepted.ActivityAPID)
	require.NotNil(t, repo.updatedTicket)
	require.NotNil(t, repo.updatedTicket.ObjectTicket)
	assert.Equal(t, "https://remote.example/tickets/1", repo.updatedTicket.ObjectTicket.ID)
	assert.Equal(t, "Updated remote ticket", repo.updatedTicket.ObjectTicket.Name)
	assert.True(t, repo.updatedTicket.ObjectTicket.HasName)
	assert.Equal(t, "done", repo.updatedTicket.ObjectTicket.Status)
	assert.True(t, repo.updatedTicket.ObjectTicket.HasStatus)
	assert.True(t, repo.updatedTicket.ObjectTicket.IsResolved)
	assert.True(t, repo.updatedTicket.ObjectTicket.HasIsResolved)
	require.NotNil(t, repo.updatedTicket.ObjectAPID)
	assert.Equal(t, "https://remote.example/tickets/1", *repo.updatedTicket.ObjectAPID)
}

func TestServiceReceiveRejectsProjectUpdateTicketStatusConflict(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/update-ticket-1","type":"Update","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/tickets/1","type":"forge:Ticket","attributedTo":"https://remote.example/users/alice","context":"http://localhost:8080/projects/project-1","forge:status":"done","forge:isResolved":false}}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrInvalidActivity)
}

func TestServiceReceiveRejectsProjectUpdateTicketWithoutFields(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/update-ticket-1","type":"Update","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/tickets/1","type":"forge:Ticket","attributedTo":"https://remote.example/users/alice","context":"http://localhost:8080/projects/project-1"}}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrInvalidActivity)
}

func TestServiceReceiveStoresProjectAddTicketAssignee(t *testing.T) {
	repo := &memoryRepository{targetActorID: "project-actor", projectActor: true}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/add-assignee-1","type":"Add","actor":"https://remote.example/users/alice","object":"http://localhost:8080/users/bob","target":"https://remote.example/tickets/1"}`)

	accepted, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.NoError(t, err)
	assert.Equal(t, "https://remote.example/activities/add-assignee-1", accepted.ActivityAPID)
	require.NotNil(t, repo.assignedTicket)
	require.NotNil(t, repo.assignedTicket.ObjectAPID)
	require.NotNil(t, repo.assignedTicket.TargetAPID)
	assert.Equal(t, "http://localhost:8080/users/bob", *repo.assignedTicket.ObjectAPID)
	assert.Equal(t, "https://remote.example/tickets/1", *repo.assignedTicket.TargetAPID)
}

func TestServiceReceiveRejectsProjectAddTicketAssigneeWithoutTarget(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/add-assignee-1","type":"Add","actor":"https://remote.example/users/alice","object":"http://localhost:8080/users/bob"}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrInvalidActivity)
}

func TestServiceReceiveRejectsProjectAddTicketAssigneeProjectTarget(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/add-assignee-1","type":"Add","actor":"https://remote.example/users/alice","object":"http://localhost:8080/users/bob","target":"http://localhost:8080/projects/project-1"}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrInvalidActivity)
}

func TestServiceReceiveStoresProjectRemoveTicketAssignee(t *testing.T) {
	repo := &memoryRepository{targetActorID: "project-actor", projectActor: true}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/remove-assignee-1","type":"Remove","actor":"https://remote.example/users/alice","object":"http://localhost:8080/users/bob","target":"https://remote.example/tickets/1"}`)

	accepted, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.NoError(t, err)
	assert.Equal(t, "https://remote.example/activities/remove-assignee-1", accepted.ActivityAPID)
	require.NotNil(t, repo.unassignedTicket)
	require.NotNil(t, repo.unassignedTicket.ObjectAPID)
	require.NotNil(t, repo.unassignedTicket.TargetAPID)
	assert.Equal(t, "http://localhost:8080/users/bob", *repo.unassignedTicket.ObjectAPID)
	assert.Equal(t, "https://remote.example/tickets/1", *repo.unassignedTicket.TargetAPID)
}

func TestServiceReceiveRejectsProjectRemoveTicketAssigneeWithoutTarget(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/remove-assignee-1","type":"Remove","actor":"https://remote.example/users/alice","object":"http://localhost:8080/users/bob"}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrInvalidActivity)
}

func TestServiceReceiveStoresProjectDeleteTicket(t *testing.T) {
	repo := &memoryRepository{targetActorID: "project-actor", projectActor: true}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/delete-ticket-1","type":"Delete","actor":"https://remote.example/users/alice","object":"https://remote.example/tickets/1","target":"http://localhost:8080/projects/project-1"}`)

	accepted, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.NoError(t, err)
	assert.Equal(t, "https://remote.example/activities/delete-ticket-1", accepted.ActivityAPID)
	require.NotNil(t, repo.deletedTicket)
	require.NotNil(t, repo.deletedTicket.ObjectAPID)
	require.NotNil(t, repo.deletedTicket.TargetAPID)
	assert.Equal(t, "https://remote.example/tickets/1", *repo.deletedTicket.ObjectAPID)
	assert.Equal(t, "http://localhost:8080/projects/project-1", *repo.deletedTicket.TargetAPID)
}

func TestServiceReceiveRejectsProjectDeleteTicketWithoutObject(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/delete-ticket-1","type":"Delete","actor":"https://remote.example/users/alice","target":"http://localhost:8080/projects/project-1"}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrInvalidActivity)
}

func TestServiceReceiveRejectsProjectDeleteTicketProjectObject(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/delete-ticket-1","type":"Delete","actor":"https://remote.example/users/alice","object":"http://localhost:8080/projects/project-1"}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrInvalidActivity)
}

func TestServiceReceiveStoresProjectAcceptInvite(t *testing.T) {
	repo := &memoryRepository{targetActorID: "project-actor", projectActor: true}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/accept-invite-1","type":"Accept","actor":"https://remote.example/users/alice","object":"http://localhost:8080/activities/invite-1","target":"http://localhost:8080/projects/project-1"}`)

	accepted, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.NoError(t, err)
	assert.Equal(t, "https://remote.example/activities/accept-invite-1", accepted.ActivityAPID)
	require.NotNil(t, repo.acceptedInvite)
	require.NotNil(t, repo.acceptedInvite.ObjectAPID)
	require.NotNil(t, repo.acceptedInvite.TargetAPID)
	assert.Equal(t, "http://localhost:8080/activities/invite-1", *repo.acceptedInvite.ObjectAPID)
	assert.Equal(t, "http://localhost:8080/projects/project-1", *repo.acceptedInvite.TargetAPID)
}

func TestServiceReceiveStoresProjectRejectInvite(t *testing.T) {
	repo := &memoryRepository{targetActorID: "project-actor", projectActor: true}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/reject-invite-1","type":"Reject","actor":"https://remote.example/users/alice","object":{"id":"http://localhost:8080/activities/invite-1","type":"Invite","actor":"http://localhost:8080/users/owner","object":"http://localhost:8080/projects/project-1"},"target":"http://localhost:8080/projects/project-1"}`)

	accepted, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.NoError(t, err)
	assert.Equal(t, "https://remote.example/activities/reject-invite-1", accepted.ActivityAPID)
	require.NotNil(t, repo.rejectedInvite)
	require.NotNil(t, repo.rejectedInvite.ObjectAPID)
	require.NotNil(t, repo.rejectedInvite.ObjectActivity)
	assert.Equal(t, "http://localhost:8080/activities/invite-1", *repo.rejectedInvite.ObjectAPID)
	assert.Equal(t, "Invite", repo.rejectedInvite.ObjectActivity.Type)
}

func TestServiceReceiveRejectsProjectInviteResponseWrongTarget(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/accept-invite-1","type":"Accept","actor":"https://remote.example/users/alice","object":"http://localhost:8080/activities/invite-1","target":"http://localhost:8080/projects/other"}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrInvalidActivity)
}

func TestServiceReceiveRejectsProjectInviteResponseWrongEmbeddedObject(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/accept-follow-1","type":"Accept","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/activities/follow-1","type":"Follow","actor":"http://localhost:8080/projects/project-1","object":"https://remote.example/users/alice"},"target":"http://localhost:8080/projects/project-1"}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrInvalidActivity)
}

func TestServiceReceiveUndoProjectFollow(t *testing.T) {
	repo := &memoryRepository{targetActorID: "project-actor", projectActor: true}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/undo-follow-1","type":"Undo","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/activities/follow-1","type":"Follow","actor":"https://remote.example/users/alice","object":"http://localhost:8080/projects/project-1"}}`)

	accepted, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.NoError(t, err)
	assert.Equal(t, "https://remote.example/activities/undo-follow-1", accepted.ActivityAPID)
	require.NotNil(t, repo.undo)
	assert.Equal(t, "Undo", repo.undo.Type)
	require.NotNil(t, repo.undo.ObjectActivity)
	assert.Equal(t, "Follow", repo.undo.ObjectActivity.Type)
	require.NotNil(t, repo.undo.ObjectAPID)
	assert.Equal(t, "https://remote.example/activities/follow-1", *repo.undo.ObjectAPID)
	require.NotNil(t, repo.undo.TargetAPID)
	assert.Equal(t, "http://localhost:8080/projects/project-1", *repo.undo.TargetAPID)
}

func TestServiceReceiveRejectsUndoFollowActorMismatch(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/undo-follow-1","type":"Undo","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/activities/follow-1","type":"Follow","actor":"https://remote.example/users/bob","object":"http://localhost:8080/projects/project-1"}}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrForbiddenActor)
}

func TestServiceReceiveRejectsUndoFollowWrongObject(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "project-actor", projectActor: true}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/undo-follow-1","type":"Undo","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/activities/follow-1","type":"Follow","actor":"https://remote.example/users/alice","object":"http://localhost:8080/projects/other"}}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/projects/project-1", body)

	require.ErrorIs(t, err, ErrInvalidActivity)
}

func TestServiceReceiveRejectsSignatureActorMismatch(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "target-actor"}, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}})
	body := []byte(`{"id":"https://remote.example/activities/1","type":"Create","actor":"https://remote.example/users/mallory"}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/users/bob", body)

	require.ErrorIs(t, err, ErrForbiddenActor)
}

func TestServiceReceiveMapsVerifierFailureToUnauthorized(t *testing.T) {
	service := NewService(&memoryRepository{targetActorID: "target-actor"}, fakeVerifier{err: errors.New("bad signature")})
	body := []byte(`{"id":"https://remote.example/activities/1","type":"Create","actor":"https://remote.example/users/alice"}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/users/bob", body)

	require.ErrorIs(t, err, ErrUnauthorized)
}

func TestServiceReceiveMapsMissingTargetToNotFound(t *testing.T) {
	service := NewService(&memoryRepository{findErr: sql.ErrNoRows}, fakeVerifier{})
	body := []byte(`{"id":"https://remote.example/activities/1","type":"Create","actor":"https://remote.example/users/alice"}`)

	_, err := service.Receive(context.Background(), newInboxRequest(t, string(body)), "http://localhost:8080/users/bob", body)

	require.ErrorIs(t, err, ErrTargetNotFound)
}

func TestParseActivityValidation(t *testing.T) {
	t.Run("unsupported type", func(t *testing.T) {
		_, err := parseActivity([]byte(`{"id":"https://remote.example/activities/1","type":"Like","actor":"https://remote.example/users/alice"}`))
		require.ErrorIs(t, err, ErrUnsupportedActivity)
	})

	t.Run("invalid actor", func(t *testing.T) {
		_, err := parseActivity([]byte(`{"id":"https://remote.example/activities/1","type":"Create","actor":"alice"}`))
		require.ErrorIs(t, err, ErrInvalidActivity)
	})

	t.Run("type array", func(t *testing.T) {
		activity, err := parseActivity([]byte(`{"id":"https://remote.example/activities/1","type":["Create","Activity"],"actor":"https://remote.example/users/alice"}`))
		require.NoError(t, err)
		assert.Equal(t, "Create", activity.Type)
	})
}

func newInboxRequest(t *testing.T, body string) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, "http://localhost:8080/users/bob/inbox", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/activity+json")
	return req
}
