package remoteinbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerReceiveUserInbox(t *testing.T) {
	repo := &memoryRepository{targetActorID: "target-actor"}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}}, WithMaxBodyBytes(2048))
	handler := NewHandler(service, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	e := echo.New()
	handler.RegisterRoutes(e)

	body := `{"id":"https://remote.example/activities/1","type":"Follow","actor":"https://remote.example/users/alice","object":"http://localhost:8080/users/bob"}`
	req := httptest.NewRequest(http.MethodPost, "/users/bob/inbox", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/activity+json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), "application/json")
	require.NotNil(t, repo.stored)
	assert.Equal(t, "Follow", repo.stored.Type)
	assert.Equal(t, "https://remote.example/activities/1", jsonField(t, rec.Body.String(), "activity_ap_id"))
	assert.Equal(t, "stored-activity", jsonField(t, rec.Body.String(), "activity_id"))
	assert.NotEmpty(t, jsonField(t, rec.Body.String(), "received_at"))
}

func TestHandlerReceiveProjectInbox(t *testing.T) {
	repo := &memoryRepository{
		targetActorID: "project-actor",
		projectActor:  true,
	}
	service := NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}}, WithMaxBodyBytes(2048))
	handler := NewHandler(service, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	e := echo.New()
	handler.RegisterRoutes(e)

	body := `{"id":"https://remote.example/activities/project-follow","type":"Follow","actor":"https://remote.example/users/alice","object":"http://localhost:8080/projects/project-1"}`
	req := httptest.NewRequest(http.MethodPost, "/projects/project-1/inbox", strings.NewReader(body))
	req.Header.Set("Content-Type", `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), "application/json")
	require.NotNil(t, repo.stored)
	assert.Equal(t, "Follow", repo.stored.Type)
	assert.Equal(t, "https://remote.example/activities/project-follow", jsonField(t, rec.Body.String(), "activity_ap_id"))
	assert.NotContains(t, rec.Body.String(), "response_activity_id")
}

func TestHandlerRejectsUnsupportedMediaType(t *testing.T) {
	handler := NewHandler(NewService(&memoryRepository{}, fakeVerifier{}), activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	e := echo.New()
	handler.RegisterRoutes(e)

	req := httptest.NewRequest(http.MethodPost, "/users/bob/inbox", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	assert.JSONEq(t, `{"error":"unsupported inbox media type"}`, rec.Body.String())
}

func TestHandlerRejectsMissingContentType(t *testing.T) {
	repo := &memoryRepository{targetActorID: "target-actor"}
	handler := NewHandler(NewService(repo, fakeVerifier{}), activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	e := echo.New()
	handler.RegisterRoutes(e)

	req := httptest.NewRequest(http.MethodPost, "/users/bob/inbox", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	assert.JSONEq(t, `{"error":"unsupported inbox media type"}`, rec.Body.String())
	assert.Nil(t, repo.stored)
}

func TestHandlerRejectsOversizedBody(t *testing.T) {
	repo := &memoryRepository{targetActorID: "target-actor"}
	handler := NewHandler(NewService(repo, fakeVerifier{}, WithMaxBodyBytes(4)), activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	e := echo.New()
	handler.RegisterRoutes(e)

	req := httptest.NewRequest(http.MethodPost, "/users/bob/inbox", strings.NewReader(`{"too":"large"}`))
	req.Header.Set("Content-Type", "application/activity+json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.JSONEq(t, `{"error":"inbox activity body too large"}`, rec.Body.String())
	assert.Nil(t, repo.stored)
}

func TestHandlerRejectsBodyReadError(t *testing.T) {
	handler := NewHandler(NewService(&memoryRepository{}, fakeVerifier{}, WithMaxBodyBytes(2048)), activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	e := echo.New()
	handler.RegisterRoutes(e)

	req := httptest.NewRequest(http.MethodPost, "/users/bob/inbox", strings.NewReader(""))
	req.Body = io.NopCloser(errReader{})
	req.Header.Set("Content-Type", "application/activity+json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.JSONEq(t, `{"error":"failed to read inbox activity"}`, rec.Body.String())
}

func TestHandlerMapsUnauthorized(t *testing.T) {
	repo := &memoryRepository{targetActorID: "target-actor"}
	handler := NewHandler(
		NewService(
			repo,
			fakeVerifier{err: context.Canceled},
		),
		activitypub.NewConfig("http://localhost:8080", "localhost:8080"),
	)
	e := echo.New()
	handler.RegisterRoutes(e)

	body := `{"id":"https://remote.example/activities/1","type":"Follow","actor":"https://remote.example/users/alice"}`
	req := httptest.NewRequest(http.MethodPost, "/users/bob/inbox", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/activity+json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.JSONEq(t, `{"error":"unauthorized inbox activity"}`, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "context canceled")
	assert.Nil(t, repo.stored)
}

func TestHandlerRejectsMalformedActivityWithoutStore(t *testing.T) {
	repo := &memoryRepository{targetActorID: "target-actor"}
	handler := NewHandler(NewService(repo, fakeVerifier{verified: &httpsig.VerifiedRequest{
		ActorID:   "remote-actor",
		ActorAPID: "https://remote.example/users/alice",
	}}), activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	e := echo.New()
	handler.RegisterRoutes(e)

	req := httptest.NewRequest(http.MethodPost, "/users/bob/inbox", strings.NewReader(`{"id":`))
	req.Header.Set("Content-Type", "application/activity+json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.JSONEq(t, `{"error":"invalid inbox activity"}`, rec.Body.String())
	assert.Nil(t, repo.stored)
}

func TestHandlerMapsInboxErrorsToStableResponses(t *testing.T) {
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	cases := []struct {
		name       string
		service    *Service
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "target not found",
			service:    NewService(&memoryRepository{findErr: sql.ErrNoRows}, fakeVerifier{}),
			body:       `{"id":"https://remote.example/activities/1","type":"Follow","actor":"https://remote.example/users/alice"}`,
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":"inbox target actor not found"}`,
		},
		{
			name: "forbidden actor",
			service: NewService(&memoryRepository{targetActorID: "target-actor"}, fakeVerifier{verified: &httpsig.VerifiedRequest{
				ActorID:   "remote-actor",
				ActorAPID: "https://remote.example/users/alice",
			}}),
			body:       `{"id":"https://remote.example/activities/1","type":"Follow","actor":"https://remote.example/users/mallory"}`,
			wantStatus: http.StatusForbidden,
			wantBody:   `{"error":"activity actor does not match signature actor"}`,
		},
		{
			name: "invalid activity",
			service: NewService(&memoryRepository{targetActorID: "target-actor"}, fakeVerifier{verified: &httpsig.VerifiedRequest{
				ActorID:   "remote-actor",
				ActorAPID: "https://remote.example/users/alice",
			}}),
			body:       `{"id":"https://remote.example/activities/1","type":"Follow","actor":"alice"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid inbox activity"}`,
		},
		{
			name: "unsupported activity",
			service: NewService(&memoryRepository{targetActorID: "target-actor"}, fakeVerifier{verified: &httpsig.VerifiedRequest{
				ActorID:   "remote-actor",
				ActorAPID: "https://remote.example/users/alice",
			}}),
			body:       `{"id":"https://remote.example/activities/1","type":"Like","actor":"https://remote.example/users/alice"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"error":"unsupported inbox activity type"}`,
		},
		{
			name: "activity conflict",
			service: NewService(&memoryRepository{
				targetActorID: "target-actor",
				storeErr:      ErrActivityConflict,
			}, fakeVerifier{verified: &httpsig.VerifiedRequest{
				ActorID:   "remote-actor",
				ActorAPID: "https://remote.example/users/alice",
			}}),
			body:       `{"id":"https://remote.example/activities/1","type":"Create","actor":"https://remote.example/users/alice","object":"https://remote.example/objects/1"}`,
			wantStatus: http.StatusConflict,
			wantBody:   `{"error":"inbox activity conflicts with stored activity"}`,
		},
		{
			name: "internal error",
			service: NewService(&memoryRepository{
				targetActorID: "target-actor",
				storeErr:      errors.New("database failed near private_key_pem"),
			}, fakeVerifier{verified: &httpsig.VerifiedRequest{
				ActorID:   "remote-actor",
				ActorAPID: "https://remote.example/users/alice",
			}}),
			body:       `{"id":"https://remote.example/activities/1","type":"Create","actor":"https://remote.example/users/alice","object":"https://remote.example/objects/1"}`,
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"failed to receive inbox activity"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewHandler(tc.service, cfg)
			e := echo.New()
			handler.RegisterRoutes(e)
			req := httptest.NewRequest(http.MethodPost, "/users/bob/inbox", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/activity+json")
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.JSONEq(t, tc.wantBody, rec.Body.String())
			assert.NotContains(t, rec.Body.String(), "private_key_pem")
		})
	}
}

type errReader struct{}

func (errReader) Read(p []byte) (int, error) {
	return 0, errors.New("socket read failed")
}

func jsonField(t *testing.T, raw, key string) string {
	t.Helper()

	var response map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &response))
	value, ok := response[key].(string)
	require.True(t, ok, "missing string field %s in %s", key, raw)
	return value
}
