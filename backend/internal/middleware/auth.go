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

// JWTMiddleware authenticates Bearer JWTs and optionally validates token versions.
func JWTMiddleware(secret []byte, validators ...TokenVersionValidator) echo.MiddlewareFunc {
	var validator TokenVersionValidator
	if len(validators) > 0 {
		validator = validators[0]
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {

		return func(c echo.Context) error {
			tokenString, err := requestToken(c.Request())
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
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

				return next(c)
			}

			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token claims"})
		}
	}
}

// requestToken prefers an explicit bearer credential and otherwise accepts the
// HttpOnly browser session cookie.
func requestToken(req *http.Request) (string, error) {
	authHeader := strings.TrimSpace(req.Header.Get("Authorization"))
	if authHeader != "" {
		headerParts := strings.Fields(authHeader)
		if len(headerParts) != 2 || headerParts[0] != "Bearer" || headerParts[1] == "" {
			return "", errors.New("invalid authorization header format")
		}
		return headerParts[1], nil
	}
	if token, ok := authsession.TokenFromRequest(req); ok {
		return token, nil
	}
	return "", errors.New("missing authentication credential")
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
