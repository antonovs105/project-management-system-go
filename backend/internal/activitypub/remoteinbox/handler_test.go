package remoteinbox

import (
	"context"
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
	require.NotNil(t, repo.stored)
	assert.Equal(t, "Follow", repo.stored.Type)
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
}

func TestHandlerRejectsOversizedBody(t *testing.T) {
	handler := NewHandler(NewService(&memoryRepository{}, fakeVerifier{}, WithMaxBodyBytes(4)), activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	e := echo.New()
	handler.RegisterRoutes(e)

	req := httptest.NewRequest(http.MethodPost, "/users/bob/inbox", strings.NewReader(`{"too":"large"}`))
	req.Header.Set("Content-Type", "application/activity+json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestHandlerMapsUnauthorized(t *testing.T) {
	handler := NewHandler(
		NewService(
			&memoryRepository{targetActorID: "target-actor"},
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
}
