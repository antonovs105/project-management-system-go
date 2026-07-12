package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// TrustedOriginMiddleware rejects browser state-changing requests from origins
// outside the configured frontend allowlist. Requests without an Origin header
// remain available to non-browser API clients.
func TrustedOriginMiddleware(origins []string) echo.MiddlewareFunc {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		origin = normalizeOrigin(origin)
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if isSafeMethod(c.Request().Method) {
				return next(c)
			}
			origin := normalizeOrigin(c.Request().Header.Get(echo.HeaderOrigin))
			if origin == "" {
				return next(c)
			}
			if _, ok := allowed[origin]; !ok {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "untrusted request origin"})
			}
			return next(c)
		}
	}
}

// normalizeOrigin produces the stable representation used by configured CORS origins.
func normalizeOrigin(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "/"))
}

// isSafeMethod reports methods that do not mutate application state.
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
