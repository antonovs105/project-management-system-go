//go:build integration

package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivityPubReadContentNegotiation(t *testing.T) {
	db := openIntegrationDB(t)
	resetIntegrationDB(t, db)

	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	userService := user.NewService(user.NewRepository(db, cfg), []byte("integration-secret"), cfg)
	createdUser, err := userService.RegisterUser(t.Context(), "http-conformance", "http-conformance@example.test", "password123")
	require.NoError(t, err)

	e := echo.New()
	activitypub.NewHandler(db, cfg).RegisterRoutes(e)
	path := "/users/" + createdUser.Username

	for _, accept := range []string{
		"",
		"*/*",
		"application/*",
		"application/json",
		activitypub.ActivityJSONMediaType,
		`application/ld+json; profile="https://www.w3.org/ns/activitystreams"`,
		"application/xml;q=0, application/activity+json;q=1",
	} {
		t.Run("accepts "+accept, func(t *testing.T) {
			rec := activityPubGET(t, e, path, accept)

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Header().Get(echo.HeaderContentType), activitypub.ActivityJSONMediaType)
		})
	}

	for _, accept := range []string{
		"application/xml",
		"text/html",
		"application/activity+json;q=0",
	} {
		t.Run("rejects "+accept, func(t *testing.T) {
			rec := activityPubGET(t, e, path, accept)

			require.Equal(t, http.StatusNotAcceptable, rec.Code, rec.Body.String())
		})
	}
}

func activityPubGET(t *testing.T, e *echo.Echo, path, accept string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	if accept != "" {
		req.Header.Set(echo.HeaderAccept, accept)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}
