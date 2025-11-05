package user

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// Depends on service to call business logic
type Handler struct {
	service *Service
}

// constructor for UserHandler.
func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}

// parsing register request
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Register method for POST /register.
func (h *Handler) Register(c echo.Context) error {
	// Parsing and validation
	var req RegisterRequest

	// c.Bind(&req) reads HTTP query body, parses json, fills struct req fields
	if err := c.Bind(&req); err != nil {
		// if json incorrect sending 400 Bad Request.
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
	}

	// TODO: add field validation

	// business logic calls
	// sending data to UserService
	// c.Request().Context() to get context.Context from query
	newUser, err := h.service.RegisterUser(c.Request().Context(), req.Username, req.Email, req.Password)
	if err != nil {
		// if service returned error sending 500 Internal Server Error.
		// TODO: add error types for more clarity
		log.Printf("Error registering user: %v", err) // Logging error
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Could not register user"})
	}

	// Success respond 201 Created
	return c.JSON(http.StatusCreated, newUser)
}
