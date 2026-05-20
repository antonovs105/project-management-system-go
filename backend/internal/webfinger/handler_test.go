package webfinger

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerResolveNegotiatesJRDResponse(t *testing.T) {
	service := NewService(mockRepository{
		actor: &ActorResource{
			Username: "alice",
			Handle:   "alice@example.test",
			APID:     "https://example.test/users/alice",
		},
	}, activitypub.NewConfig("https://example.test", "example.test"))

	for _, accept := range []string{
		"",
		"*/*",
		"application/*",
		"application/json",
		jrdMediaType,
		"application/xml;q=0, application/jrd+json;q=1",
	} {
		t.Run("accepts "+accept, func(t *testing.T) {
			rec := resolveWebFinger(t, service, accept)

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Header().Get(echo.HeaderContentType), jrdMediaType)
		})
	}

	for _, accept := range []string{
		"application/xml",
		"text/html",
		"application/jrd+json;q=0",
	} {
		t.Run("rejects "+accept, func(t *testing.T) {
			rec := resolveWebFinger(t, service, accept)

			require.Equal(t, http.StatusNotAcceptable, rec.Code, rec.Body.String())
		})
	}
}

func resolveWebFinger(t *testing.T, service *Service, accept string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	NewHandler(service).RegisterRoutes(e)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/webfinger?resource=acct:alice@example.test", nil)
	if accept != "" {
		req.Header.Set(echo.HeaderAccept, accept)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}
