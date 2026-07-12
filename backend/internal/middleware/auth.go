package middleware

import (
	"context"
	"errors"
	"log"
	"math"
	"net/http"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/authsession"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

// TokenVersionValidator validates whether a JWT token_version claim is still current.
type TokenVersionValidator interface {
	ValidateTokenVersion(ctx context.Context, userID string, tokenVersion int) error
}

// SessionValidator validates server-side browser session identifiers.
type SessionValidator interface {
	ValidateSession(ctx context.Context, userID, sessionID string) error
}

// CredentialAuthenticator validates a non-JWT bearer credential and returns its owner and scopes.
type CredentialAuthenticator interface {
	AuthenticateCredential(ctx context.Context, raw string) (userID string, scopes []string, err error)
}

// JWTMiddleware authenticates Bearer JWTs and optionally validates token versions.
func JWTMiddleware(secret []byte, validators ...TokenVersionValidator) echo.MiddlewareFunc {
	var validator TokenVersionValidator
	if len(validators) > 0 {
		validator = validators[0]
	}
	return AuthenticationMiddleware(secret, validator, nil)
}

// AuthenticationMiddleware accepts browser/JWT sessions and scoped API credentials.
func AuthenticationMiddleware(secret []byte, validator TokenVersionValidator, credentials CredentialAuthenticator) echo.MiddlewareFunc {
	var sessionValidator SessionValidator
	if validator != nil {
		sessionValidator, _ = validator.(SessionValidator)
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {

		return func(c echo.Context) error {
			tokenString, cookieCredential, err := requestToken(c.Request())
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
			}
			if credentials != nil && !cookieCredential && strings.HasPrefix(tokenString, "progo_") {
				userID, scopes, err := credentials.AuthenticateCredential(c.Request().Context(), tokenString)
				if err != nil {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
				}
				required := requiredCredentialScope(c.Request().Method, c.Path())
				if !containsScope(scopes, required) {
					return c.JSON(http.StatusForbidden, map[string]string{"error": "api token is missing required scope", "code": "insufficient_scope"})
				}
				c.Set("userID", userID)
				c.Set("authType", "api_token")
				c.Set("authScopes", scopes)
				c.Set("mfaEnrollmentRequired", false)
				return next(c)
			}
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				if token.Method == nil || token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
					return nil, echo.NewHTTPError(http.StatusUnauthorized, "Unexpected signing method")
				}
				return secret, nil
			}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(authsession.Issuer), jwt.WithAudience(authsession.Audience))

			if err != nil {
				log.Printf("Error parsing token: %v", err)
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			}

			if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {

				userID, ok := claims["sub"].(string)
				if !ok || userID == "" {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid user id in token"})
				}

				c.Set("userID", userID)
				if validator != nil {
					tokenVersion, ok := tokenVersionFromClaims(claims)
					if !ok {
						return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
					}
					if err := validator.ValidateTokenVersion(c.Request().Context(), userID, tokenVersion); err != nil {
						return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
					}
					c.Set("tokenVersion", tokenVersion)
				}
				sessionID, _ := claims["sid"].(string)
				if cookieCredential && strings.TrimSpace(sessionID) == "" && sessionValidator != nil {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
				}
				if sessionValidator != nil && strings.TrimSpace(sessionID) != "" {
					if err := sessionValidator.ValidateSession(c.Request().Context(), userID, sessionID); err != nil {
						return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
					}
					c.Set("sessionID", sessionID)
				}
				mfaEnrollmentRequired, _ := claims["mfa_enrollment_required"].(bool)
				c.Set("mfaEnrollmentRequired", mfaEnrollmentRequired)
				c.Set("authType", "jwt")

				return next(c)
			}

			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token claims"})
		}
	}
}

func requiredCredentialScope(method, path string) string {
	if strings.HasPrefix(path, "/api/v1/admin") {
		return "admin"
	}
	if strings.HasPrefix(path, "/api/v1/me/api-tokens") {
		return "tokens:manage"
	}
	readOnly := method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
	if strings.HasPrefix(path, "/api/v1/me") {
		if readOnly {
			return "account:read"
		}
		return "account:write"
	}
	if readOnly {
		return "projects:read"
	}
	return "projects:write"
}

func containsScope(scopes []string, expected string) bool {
	for _, scope := range scopes {
		if scope == expected {
			return true
		}
	}
	return false
}

// requestToken prefers an explicit bearer credential and otherwise accepts the
// HttpOnly browser session cookie.
func requestToken(req *http.Request) (string, bool, error) {
	authHeader := strings.TrimSpace(req.Header.Get("Authorization"))
	if authHeader != "" {
		headerParts := strings.Fields(authHeader)
		if len(headerParts) != 2 || headerParts[0] != "Bearer" || headerParts[1] == "" {
			return "", false, errors.New("invalid authorization header format")
		}
		return headerParts[1], false, nil
	}
	if token, ok := authsession.TokenFromRequest(req); ok {
		return token, true, nil
	}
	return "", false, errors.New("missing authentication credential")
}

// tokenVersionFromClaims reads the token_version claim without accepting fractional values.
func tokenVersionFromClaims(claims jwt.MapClaims) (int, bool) {
	raw, ok := claims["token_version"]
	if !ok {
		return 0, false
	}
	switch value := raw.(type) {
	case float64:
		maxInt := int(^uint(0) >> 1)
		if value <= 0 || math.Trunc(value) != value || value > float64(maxInt) {
			return 0, false
		}
		return int(value), true
	case int:
		return value, value > 0
	case int64:
		maxInt := int64(int(^uint(0) >> 1))
		if value <= 0 || value > maxInt {
			return 0, false
		}
		return int(value), true
	default:
		return 0, false
	}
}
