package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// MFAEnrollmentValidator checks whether a formerly restricted account has completed enrollment.
type MFAEnrollmentValidator interface {
	ValidateMFAEnrollment(ctx context.Context, userID string) error
}

// MFAEnrollmentMiddleware restricts unenrolled privileged sessions to the
// small account surface required to activate their second factor.
func MFAEnrollmentMiddleware(validators ...MFAEnrollmentValidator) echo.MiddlewareFunc {
	var validator MFAEnrollmentValidator
	if len(validators) > 0 {
		validator = validators[0]
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			required, _ := c.Get("mfaEnrollmentRequired").(bool)
			if required && validator != nil {
				userID, _ := c.Get("userID").(string)
				required = validator.ValidateMFAEnrollment(c.Request().Context(), userID) != nil
			}
			if !required || isMFAEnrollmentRoute(c.Request().Method, c.Path()) {
				return next(c)
			}
			return c.JSON(http.StatusForbidden, map[string]string{
				"error": "multi-factor authentication enrollment is required",
				"code":  "mfa_enrollment_required",
			})
		}
	}
}

// isMFAEnrollmentRoute identifies the account routes safe for a restricted privileged session.
func isMFAEnrollmentRoute(method, path string) bool {
	path = strings.TrimSpace(path)
	if method == http.MethodGet && (path == "/api/v1/me" || path == "/api/v1/me/mfa") {
		return true
	}
	if method == http.MethodPost && (path == "/api/v1/me/logout" || path == "/api/v1/me/mfa/setup" || path == "/api/v1/me/mfa/confirm") {
		return true
	}
	return false
}
