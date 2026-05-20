package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTMiddlewareStableErrorResponses(t *testing.T) {
	secret := []byte("secret")

	cases := []struct {
		name          string
		authorization string
		wantBody      string
	}{
		{
			name:     "missing header",
			wantBody: `{"error":"missing authorization header"}`,
		},
		{
			name:          "invalid format",
			authorization: "Token abc",
			wantBody:      `{"error":"invalid authorization header format"}`,
		},
		{
			name:          "invalid token",
			authorization: "Bearer nope",
			wantBody:      `{"error":"invalid token"}`,
		},
		{
			name:          "missing subject",
			authorization: "Bearer " + signedTestToken(t, secret, jwt.MapClaims{"exp": 9999999999}),
			wantBody:      `{"error":"invalid user id in token"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := runJWTMiddleware(t, secret, tc.authorization)

			require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
			assert.JSONEq(t, tc.wantBody, rec.Body.String())
		})
	}
}

func TestJWTMiddlewareSetsUserID(t *testing.T) {
	secret := []byte("secret")
	rec := runJWTMiddleware(t, secret, "Bearer "+signedTestToken(t, secret, jwt.MapClaims{
		"sub": "user-1",
		"exp": 9999999999,
	}))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"user_id":"user-1"}`, rec.Body.String())
}

func runJWTMiddleware(t *testing.T, secret []byte, authorization string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	if authorization != "" {
		req.Header.Set(echo.HeaderAuthorization, authorization)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := JWTMiddleware(secret)(func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"user_id": c.Get("userID").(string)})
	})
	require.NoError(t, handler(c))
	return rec
}

func signedTestToken(t *testing.T, secret []byte, claims jwt.MapClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, err := token.SignedString(secret)
	require.NoError(t, err)
	return raw
}
