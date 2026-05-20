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
	targetActorID string
	actorAPID     string
	stored        *InboundActivity
	storedNote    *InboundActivity
	storeResult   *AcceptedActivity
	projectActor  bool
	follow        *InboundActivity
	followResult  *FollowResponse
	undo          *InboundActivity
	findErr       error
	storeErr      error
	storeNoteErr  error
	followErr     error
	undoErr       error
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
	targetInboxURL string
	err            error
}

func (f *fakeDelivery) Enqueue(ctx context.Context, activityID string, targetInboxURL string) (*delivery.Delivery, error) {
	f.activityID = activityID
	f.targetInboxURL = targetInboxURL
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

func TestServiceReceiveAcceptsProjectFollowAndQueuesResponse(t *testing.T) {
	repo := &memoryRepository{
		targetActorID: "project-actor",
		projectActor:  true,
		followResult: &FollowResponse{
			ActivityID:     "accept-activity",
			ActivityAPID:   "http://localhost:8080/activities/accept-activity",
			TargetInboxURL: "https://remote.example/users/alice/inbox",
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
	require.NotNil(t, repo.follow)
	assert.Equal(t, "Follow", repo.follow.Type)
	assert.Equal(t, "accept-activity", accepted.ResponseActivityID)
	assert.Equal(t, "accept-activity", delivery.activityID)
	assert.Equal(t, "https://remote.example/users/alice/inbox", delivery.targetInboxURL)
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
