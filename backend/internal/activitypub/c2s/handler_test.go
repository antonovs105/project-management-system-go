package c2s

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/comment"
	authmiddleware "github.com/antonovs105/project-management-system-go/internal/middleware"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostUserOutboxCreatesNote(t *testing.T) {
	repo := newFakeRepository()
	repo.tickets["https://local.test/tickets/1"] = "ticket-1"
	repo.activities["https://local.test/comments/1"] = &createdActivity{
		APID:     "https://local.test/activities/create-note-1",
		Document: json.RawMessage(`{"id":"https://local.test/activities/create-note-1","type":"Create","actor":"https://local.test/users/alice","object":{"id":"https://local.test/comments/1","type":"Note"}}`),
	}
	comments := &fakeCommentCreator{
		result: &comment.Comment{ID: "comment-1", APID: "https://local.test/comments/1"},
	}
	handler := NewHandlerWithRepository(repo, &fakeTicketCreator{}, comments)

	rec := postOutbox(t, handler, map[string]any{
		"type":  "Create",
		"actor": repo.user.APID,
		"object": map[string]any{
			"type":      "Note",
			"inReplyTo": "https://local.test/tickets/1",
			"content":   "Ready for review",
		},
	}, activitypub.ActivityJSONMediaType, activitypub.ActivityJSONMediaType)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, "https://local.test/activities/create-note-1", rec.Header().Get(echo.HeaderLocation))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assertVaryContains(t, rec.Header(), echo.HeaderAccept)
	assert.Equal(t, "ticket-1", comments.ticketID)
	assert.Equal(t, repo.user.ID, comments.authorID)
	assert.Equal(t, "Ready for review", comments.content)
}

func TestPostUserOutboxCreatesTicket(t *testing.T) {
	repo := newFakeRepository()
	repo.projects["https://local.test/projects/1"] = "project-1"
	repo.activities["https://local.test/tickets/created"] = &createdActivity{
		APID:     "https://local.test/activities/create-ticket-1",
		Document: json.RawMessage(`{"id":"https://local.test/activities/create-ticket-1","type":"Create","actor":"https://local.test/users/alice","object":"https://local.test/tickets/created"}`),
	}
	tickets := &fakeTicketCreator{
		result: &ticket.Ticket{ID: "ticket-1", APID: "https://local.test/tickets/created"},
	}
	handler := NewHandlerWithRepository(repo, tickets, &fakeCommentCreator{})

	rec := postOutbox(t, handler, map[string]any{
		"type":  "Create",
		"actor": repo.user.APID,
		"object": map[string]any{
			"type":             []any{"forge:Ticket"},
			"context":          "https://local.test/projects/1",
			"name":             "Implement C2S tests",
			"content":          "Cover the handler with fakes.",
			"forge:priority":   "high",
			"forge:ticketType": "task",
		},
	}, `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`, "application/json")

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, "project-1", tickets.projectID)
	assert.Equal(t, repo.user.ID, tickets.reporterID)
	assert.Equal(t, "Implement C2S tests", tickets.req.Title)
	assert.Equal(t, "high", tickets.req.Priority)
}

func TestPostUserOutboxRejectsUnsupportedContentType(t *testing.T) {
	handler := NewHandlerWithRepository(newFakeRepository(), &fakeTicketCreator{}, &fakeCommentCreator{})

	rec := postOutbox(t, handler, validNoteBody(), "application/json", activitypub.ActivityJSONMediaType)

	require.Equal(t, http.StatusUnsupportedMediaType, rec.Code, rec.Body.String())
}

func TestPostUserOutboxRejectsNotAcceptable(t *testing.T) {
	handler := NewHandlerWithRepository(newFakeRepository(), &fakeTicketCreator{}, &fakeCommentCreator{})

	rec := postOutbox(t, handler, validNoteBody(), activitypub.ActivityJSONMediaType, "application/xml")

	require.Equal(t, http.StatusNotAcceptable, rec.Code, rec.Body.String())
}

func TestPostUserOutboxRejectsZeroQualityActivityAccept(t *testing.T) {
	handler := NewHandlerWithRepository(newFakeRepository(), &fakeTicketCreator{}, &fakeCommentCreator{})

	rec := postOutbox(t, handler, validNoteBody(), activitypub.ActivityJSONMediaType, "application/activity+json;q=0")

	require.Equal(t, http.StatusNotAcceptable, rec.Code, rec.Body.String())
}

func TestPostUserOutboxRejectsClientAssignedActivityID(t *testing.T) {
	body := validNoteBody()
	body["id"] = "https://local.test/activities/client-id"
	handler := NewHandlerWithRepository(newFakeRepository(), &fakeTicketCreator{}, &fakeCommentCreator{})

	rec := postOutbox(t, handler, body, activitypub.ActivityJSONMediaType, activitypub.ActivityJSONMediaType)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "server-assigned activity id")
}

func TestPostUserOutboxRejectsMissingTicketName(t *testing.T) {
	repo := newFakeRepository()
	repo.projects["https://local.test/projects/1"] = "project-1"
	tickets := &fakeTicketCreator{}
	handler := NewHandlerWithRepository(repo, tickets, &fakeCommentCreator{})

	rec := postOutbox(t, handler, map[string]any{
		"type":  "Create",
		"actor": repo.user.APID,
		"object": map[string]any{
			"type":    "forge:Ticket",
			"context": "https://local.test/projects/1",
		},
	}, activitypub.ActivityJSONMediaType, activitypub.ActivityJSONMediaType)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "ticket name is required")
	assert.False(t, tickets.called)
}

func TestPostUserOutboxRejectsActorMismatch(t *testing.T) {
	handler := NewHandlerWithRepository(newFakeRepository(), &fakeTicketCreator{}, &fakeCommentCreator{})
	body := validNoteBody()
	body["actor"] = "https://local.test/users/bob"

	rec := postOutbox(t, handler, body, activitypub.ActivityJSONMediaType, activitypub.ActivityJSONMediaType)

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

func TestRegisterRoutesAppliesJWTMiddleware(t *testing.T) {
	secret := []byte("test-secret")

	t.Run("missing token is rejected before outbox handling", func(t *testing.T) {
		comments := &fakeCommentCreator{}
		handler := NewHandlerWithRepository(newFakeRepository(), &fakeTicketCreator{}, comments)

		rec := postOutboxThroughRoute(t, handler, secret, "", "/users/alice/outbox", validNoteBody())

		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		assert.False(t, comments.called)
	})

	t.Run("valid token reaches the outbox handler", func(t *testing.T) {
		repo := newFakeRepository()
		repo.tickets["https://local.test/tickets/1"] = "ticket-1"
		repo.activities["https://local.test/comments/created"] = &createdActivity{
			APID:     "https://local.test/activities/create-note-route",
			Document: json.RawMessage(`{"id":"https://local.test/activities/create-note-route","type":"Create","actor":"https://local.test/users/alice"}`),
		}
		comments := &fakeCommentCreator{
			result: &comment.Comment{ID: "comment-1", APID: "https://local.test/comments/created"},
		}
		handler := NewHandlerWithRepository(repo, &fakeTicketCreator{}, comments)

		rec := postOutboxThroughRoute(t, handler, secret, signedTestToken(t, secret, repo.user.ID), "/users/alice/outbox", validNoteBody())

		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
		assert.True(t, comments.called)
		assert.Equal(t, repo.user.ID, comments.authorID)
	})

	t.Run("unknown token subject is rejected", func(t *testing.T) {
		comments := &fakeCommentCreator{}
		handler := NewHandlerWithRepository(newFakeRepository(), &fakeTicketCreator{}, comments)

		rec := postOutboxThroughRoute(t, handler, secret, signedTestToken(t, secret, "missing-actor"), "/users/alice/outbox", validNoteBody())

		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		assert.False(t, comments.called)
	})

	t.Run("route username must match authenticated actor", func(t *testing.T) {
		repo := newFakeRepository()
		comments := &fakeCommentCreator{}
		handler := NewHandlerWithRepository(repo, &fakeTicketCreator{}, comments)

		rec := postOutboxThroughRoute(t, handler, secret, signedTestToken(t, secret, repo.user.ID), "/users/bob/outbox", validNoteBody())

		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		assert.False(t, comments.called)
	})
}

func TestHasType(t *testing.T) {
	assert.True(t, hasType("Create", "Create"))
	assert.True(t, hasType([]any{"Activity", "forge:Ticket"}, "forge:Ticket"))
	assert.False(t, hasType([]any{"Update"}, "Create"))
}

func TestFirstString(t *testing.T) {
	assert.Equal(t, "https://example.test/projects/1", firstString(" https://example.test/projects/1 "))
	assert.Equal(t, "https://example.test/users/alice", firstString([]any{
		"https://example.test/users/alice",
		"https://example.test/users/bob",
	}))
	assert.Empty(t, firstString([]any{42, ""}))
}

func TestDecodeObject(t *testing.T) {
	object, err := decodeObject(json.RawMessage(`{"type":"Note","content":"Ready"}`))

	require.NoError(t, err)
	assert.True(t, hasType(object["type"], "Note"))
	assert.Equal(t, "Ready", object.optionalString("content"))
}

func TestDecodeObjectRejectsNonObject(t *testing.T) {
	_, err := decodeObject(json.RawMessage(`"https://example.test/notes/1"`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an object")
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		user: &localUser{
			ID:       "actor-1",
			Username: "alice",
			APID:     "https://local.test/users/alice",
		},
		tickets:    make(map[string]string),
		projects:   make(map[string]string),
		actors:     map[string]string{"https://local.test/users/alice": "actor-1"},
		activities: make(map[string]*createdActivity),
	}
}

func validNoteBody() map[string]any {
	return map[string]any{
		"type":  "Create",
		"actor": "https://local.test/users/alice",
		"object": map[string]any{
			"type":      "Note",
			"inReplyTo": "https://local.test/tickets/1",
			"content":   "Looks good.",
		},
	}
}

func postOutbox(t *testing.T, handler *Handler, body map[string]any, contentType, accept string) *httptest.ResponseRecorder {
	t.Helper()

	raw, err := json.Marshal(body)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/users/alice/outbox", bytes.NewReader(raw))
	if contentType != "" {
		req.Header.Set(echo.HeaderContentType, contentType)
	}
	if accept != "" {
		req.Header.Set(echo.HeaderAccept, accept)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("username")
	c.SetParamValues("alice")
	c.Set("userID", "actor-1")

	require.NoError(t, handler.PostUserOutbox(c))
	return rec
}

func postOutboxThroughRoute(t *testing.T, handler *Handler, secret []byte, token, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()

	raw, err := json.Marshal(body)
	require.NoError(t, err)

	e := echo.New()
	handler.RegisterRoutes(e, authmiddleware.JWTMiddleware(secret))
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set(echo.HeaderContentType, activitypub.ActivityJSONMediaType)
	req.Header.Set(echo.HeaderAccept, activitypub.ActivityJSONMediaType)
	if token != "" {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func signedTestToken(t *testing.T, secret []byte, subject string) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": subject,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	raw, err := token.SignedString(secret)
	require.NoError(t, err)
	return raw
}

func assertVaryContains(t *testing.T, header http.Header, expected string) {
	t.Helper()

	for _, value := range header.Values(echo.HeaderVary) {
		for _, item := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(item), expected) {
				return
			}
		}
	}
	require.Failf(t, "missing Vary value", "expected Vary to contain %s, got %v", expected, header.Values(echo.HeaderVary))
}

type fakeRepository struct {
	user       *localUser
	tickets    map[string]string
	projects   map[string]string
	actors     map[string]string
	activities map[string]*createdActivity
}

func (f *fakeRepository) LocalUser(ctx context.Context, userID string) (*localUser, error) {
	if f.user != nil && f.user.ID == userID {
		return f.user, nil
	}
	return nil, errors.New("user not found")
}

func (f *fakeRepository) LocalTicketID(ctx context.Context, apID string) (string, error) {
	if id, ok := f.tickets[apID]; ok {
		return id, nil
	}
	return "", errors.New("local ticket not found")
}

func (f *fakeRepository) LocalProjectID(ctx context.Context, apID string) (string, error) {
	if id, ok := f.projects[apID]; ok {
		return id, nil
	}
	return "", errors.New("local project not found")
}

func (f *fakeRepository) ActorID(ctx context.Context, apID string) (*string, error) {
	if id, ok := f.actors[apID]; ok {
		return &id, nil
	}
	return nil, errors.New("actor not found")
}

func (f *fakeRepository) CreatedActivity(ctx context.Context, actorID, objectAPID string) (*createdActivity, error) {
	if activity, ok := f.activities[objectAPID]; ok {
		return activity, nil
	}
	return nil, errors.New("activity not found")
}

type fakeTicketCreator struct {
	called     bool
	req        ticket.CreateTicketRequest
	projectID  string
	reporterID string
	result     *ticket.Ticket
	err        error
}

func (f *fakeTicketCreator) CreateTicket(ctx context.Context, req ticket.CreateTicketRequest, projectID, reporterID string) (*ticket.Ticket, error) {
	f.called = true
	f.req = req
	f.projectID = projectID
	f.reporterID = reporterID
	if f.err != nil {
		return nil, f.err
	}
	if f.result == nil {
		return nil, errors.New("missing fake ticket")
	}
	return f.result, nil
}

type fakeCommentCreator struct {
	called   bool
	ticketID string
	authorID string
	content  string
	result   *comment.Comment
	err      error
}

func (f *fakeCommentCreator) CreateComment(ctx context.Context, ticketID, authorID, content string) (*comment.Comment, error) {
	f.called = true
	f.ticketID = ticketID
	f.authorID = authorID
	f.content = content
	if f.err != nil {
		return nil, f.err
	}
	if f.result == nil {
		return nil, errors.New("missing fake comment")
	}
	return f.result, nil
}
