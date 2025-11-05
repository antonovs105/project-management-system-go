package middleware

import (
	"log"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

// JWTMiddleware fabric for creating middleware
func JWTMiddleware(secret []byte) echo.MiddlewareFunc {

	return func(next echo.HandlerFunc) echo.HandlerFunc {

		return func(c echo.Context) error {
			// taking jwt
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Missing authorization header"})
			}

			// Expected format "Bearer <token>"
			headerParts := strings.Split(authHeader, " ")
			if len(headerParts) != 2 || headerParts[0] != "Bearer" {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid authorization header format"})
			}

			tokenString := headerParts[1]

			// parsing and validatiing
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, echo.NewHTTPError(http.StatusUnauthorized, "Unexpected signing method")
				}
				return secret, nil
			})

			if err != nil {
				log.Printf("Error parsing token: %v", err)
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid token"})
			}

			// takes data (claims) and adds it to context
			if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {

				// Claims stores data as interface{} so transform needed
				userIDFloat, ok := claims["sub"].(float64)
				if !ok {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid user ID in token"})
				}

				// transfrm float64 to int64
				userID := int64(userIDFloat)

				c.Set("userID", userID)

				// next handler in pipeline
				return next(c)
			}

			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid token claims"})
		}
	}
}
