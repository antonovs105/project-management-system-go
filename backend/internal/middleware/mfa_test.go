package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

// mfaEnrollmentValidatorStub controls live enrollment checks in middleware tests.
type mfaEnrollmentValidatorStub struct {
	err error
}

// ValidateMFAEnrollment returns the configured enrollment result.
func (s mfaEnrollmentValidatorStub) ValidateMFAEnrollment(context.Context, string) error {
	return s.err
}

func TestMFAEnrollmentMiddlewareRestrictsPrivilegedBootstrapSession(t *testing.T) {
	e := echo.New()
	e.GET("/api/v1/projects", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}, func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("userID", "user-id")
			c.Set("mfaEnrollmentRequired", true)
			return next(c)
		}
	}, MFAEnrollmentMiddleware(mfaEnrollmentValidatorStub{err: errors.New("not enrolled")}))

	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "mfa_enrollment_required")
}

func TestMFAEnrollmentMiddlewareAllowsSetupAndLiveCompletion(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		path      string
		method    string
		validator error
	}{
		{name: "setup route", path: "/api/v1/me/mfa/setup", method: http.MethodPost, validator: errors.New("not enrolled")},
		{name: "completed enrollment", path: "/api/v1/projects", method: http.MethodGet},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			e := echo.New()
			e.Add(testCase.method, testCase.path, func(c echo.Context) error { return c.NoContent(http.StatusNoContent) }, func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c echo.Context) error {
					c.Set("userID", "user-id")
					c.Set("mfaEnrollmentRequired", true)
					return next(c)
				}
			}, MFAEnrollmentMiddleware(mfaEnrollmentValidatorStub{err: testCase.validator}))
			recorder := httptest.NewRecorder()
			e.ServeHTTP(recorder, httptest.NewRequest(testCase.method, testCase.path, nil))
			require.Equal(t, http.StatusNoContent, recorder.Code)
		})
	}
}
