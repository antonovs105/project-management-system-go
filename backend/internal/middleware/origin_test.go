package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrustedOriginMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		origin     string
		wantStatus int
	}{
		{name: "allowed mutation", method: http.MethodPost, origin: "https://app.example.test", wantStatus: http.StatusNoContent},
		{name: "normalizes allowed origin", method: http.MethodPatch, origin: "HTTPS://APP.EXAMPLE.TEST/", wantStatus: http.StatusNoContent},
		{name: "rejects browser mutation", method: http.MethodDelete, origin: "https://evil.example.test", wantStatus: http.StatusForbidden},
		{name: "allows non browser client", method: http.MethodPost, wantStatus: http.StatusNoContent},
		{name: "allows safe method", method: http.MethodGet, origin: "https://evil.example.test", wantStatus: http.StatusNoContent},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(tc.method, "/api/v1/test", nil)
			if tc.origin != "" {
				req.Header.Set(echo.HeaderOrigin, tc.origin)
			}
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)
			handler := TrustedOriginMiddleware([]string{"https://app.example.test"})(func(c echo.Context) error {
				return c.NoContent(http.StatusNoContent)
			})

			require.NoError(t, handler(ctx))
			assert.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}
