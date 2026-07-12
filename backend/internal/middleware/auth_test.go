package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/authsession"
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
			wantBody: `{"error":"missing authentication credential"}`,
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

func TestJWTMiddlewareAcceptsSessionCookie(t *testing.T) {
	secret := []byte("secret")
	raw := signedTestToken(t, secret, jwt.MapClaims{"sub": "user-1", "exp": 9999999999})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(authsession.NewCookie(raw, false))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	handler := JWTMiddleware(secret)(func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"user_id": c.Get("userID").(string)})
	})

	require.NoError(t, handler(c))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestJWTMiddlewareRejectsUnexpectedHMACVariant(t *testing.T) {
	secret := []byte("secret")
	token := jwt.NewWithClaims(jwt.SigningMethodHS384, jwt.MapClaims{
		"sub": "user-1",
		"exp": 9999999999,
	})
	raw, err := token.SignedString(secret)
	require.NoError(t, err)

	rec := runJWTMiddleware(t, secret, "Bearer "+raw)

	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"error":"invalid token"}`, rec.Body.String())
}

func TestJWTMiddlewareValidatesTokenVersion(t *testing.T) {
	secret := []byte("secret")
	validator := &testTokenValidator{wantUserID: "user-1", wantVersion: 2}
	rec := runJWTMiddlewareWithValidator(t, secret, "Bearer "+signedTestToken(t, secret, jwt.MapClaims{
		"sub":           "user-1",
		"token_version": 2,
		"exp":           9999999999,
	}), validator)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, validator.called)
}

func TestJWTMiddlewareRejectsStaleTokenVersion(t *testing.T) {
	secret := []byte("secret")
	rec := runJWTMiddlewareWithValidator(t, secret, "Bearer "+signedTestToken(t, secret, jwt.MapClaims{
		"sub":           "user-1",
		"token_version": 1,
		"exp":           9999999999,
	}), &testTokenValidator{wantUserID: "user-1", wantVersion: 2, err: errors.New("stale token")})

	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"error":"invalid token"}`, rec.Body.String())
}

type credentialAuthenticatorStub struct {
	userID string
	scopes []string
	err    error
}

func (s credentialAuthenticatorStub) AuthenticateCredential(context.Context, string) (string, []string, error) {
	return s.userID, s.scopes, s.err
}

func TestAuthenticationMiddlewareEnforcesAPITokenScopes(t *testing.T) {
	e := echo.New()
	authenticator := credentialAuthenticatorStub{userID: "11111111-1111-4111-8111-111111111111", scopes: []string{"projects:read"}}
	authentication := AuthenticationMiddleware([]byte("secret"), nil, authenticator)
	e.GET("/api/v1/projects", func(c echo.Context) error {
		require.Equal(t, "api_token", c.Get("authType"))
		return c.NoContent(http.StatusNoContent)
	}, authentication)
	e.POST("/api/v1/projects", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) }, authentication)

	read := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	read.Header.Set(echo.HeaderAuthorization, "Bearer progo_test_credential")
	readRecorder := httptest.NewRecorder()
	e.ServeHTTP(readRecorder, read)
	require.Equal(t, http.StatusNoContent, readRecorder.Code)

	write := httptest.NewRequest(http.MethodPost, "/api/v1/projects", nil)
	write.Header.Set(echo.HeaderAuthorization, "Bearer progo_test_credential")
	writeRecorder := httptest.NewRecorder()
	e.ServeHTTP(writeRecorder, write)
	require.Equal(t, http.StatusForbidden, writeRecorder.Code)
	require.Contains(t, writeRecorder.Body.String(), "insufficient_scope")
}

func runJWTMiddleware(t *testing.T, secret []byte, authorization string) *httptest.ResponseRecorder {
	return runJWTMiddlewareWithValidator(t, secret, authorization, nil)
}

func runJWTMiddlewareWithValidator(t *testing.T, secret []byte, authorization string, validator TokenVersionValidator) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	if authorization != "" {
		req.Header.Set(echo.HeaderAuthorization, authorization)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := JWTMiddleware(secret, validator)(func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"user_id": c.Get("userID").(string)})
	})
	require.NoError(t, handler(c))
	return rec
}

func signedTestToken(t *testing.T, secret []byte, claims jwt.MapClaims) string {
	t.Helper()

	claims["iss"] = authsession.Issuer
	claims["aud"] = authsession.Audience
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, err := token.SignedString(secret)
	require.NoError(t, err)
	return raw
}

type testTokenValidator struct {
	wantUserID  string
	wantVersion int
	called      bool
	err         error
}

func (v *testTokenValidator) ValidateTokenVersion(ctx context.Context, userID string, tokenVersion int) error {
	v.called = true
	if userID != v.wantUserID || tokenVersion != v.wantVersion {
		return errors.New("unexpected token version")
	}
	return v.err
}
