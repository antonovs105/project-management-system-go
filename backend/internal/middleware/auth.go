package middleware

import (
	"context"
	"log"
	"math"
	"net/http"
	"strings"

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
			// taking jwt
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing authorization header"})
			}

			// Expected format "Bearer <token>"
			headerParts := strings.Split(authHeader, " ")
			if len(headerParts) != 2 || headerParts[0] != "Bearer" {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid authorization header format"})
			}

			tokenString := headerParts[1]

			// Verify the signature before trusting claims.
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, echo.NewHTTPError(http.StatusUnauthorized, "Unexpected signing method")
				}
				return secret, nil
			})

			if err != nil {
				log.Printf("Error parsing token: %v", err)
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			}

			// takes data (claims) and adds it to context
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

				// next handler in pipeline
				return next(c)
			}

			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token claims"})
		}
	}
}

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
