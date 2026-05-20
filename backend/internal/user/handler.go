package user

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

const AdminBootstrapTokenHeader = "X-Admin-Bootstrap-Token"

// Depends on service to call business logic
type Handler struct {
	service             *Service
	adminBootstrapToken string
}

// constructor for UserHandler.
func NewHandler(service *Service, adminBootstrapToken ...string) *Handler {
	token := ""
	if len(adminBootstrapToken) > 0 {
		token = strings.TrimSpace(adminBootstrapToken[0])
	}
	return &Handler{
		service:             service,
		adminBootstrapToken: token,
	}
}

func (h *Handler) RegisterRoutes(e *echo.Echo) {
	e.POST("/setup/admin", h.BootstrapAdmin)
	e.POST("/register", h.Register)
	e.POST("/login", h.Login)
}

// parsing register request
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type BootstrapAdminRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) BootstrapAdmin(c echo.Context) error {
	if h.adminBootstrapToken == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "admin bootstrap disabled"})
	}
	if !sameSecret(h.adminBootstrapToken, c.Request().Header.Get(AdminBootstrapTokenHeader)) {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid admin bootstrap token"})
	}

	var req BootstrapAdminRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	admin, err := h.service.BootstrapAdmin(c.Request().Context(), req.Username, req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidUserInput) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		if errors.Is(err, ErrAdminAlreadyExists) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "admin user already exists"})
		}
		log.Printf("Error bootstrapping admin user: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not bootstrap admin"})
	}

	return c.JSON(http.StatusCreated, admin)
}

func sameSecret(expected, actual string) bool {
	expectedHash := sha256.Sum256([]byte(expected))
	actualHash := sha256.Sum256([]byte(actual))
	return subtle.ConstantTimeCompare(expectedHash[:], actualHash[:]) == 1
}

// Register method for POST /register.
func (h *Handler) Register(c echo.Context) error {
	// Parsing and validation
	var req RegisterRequest

	// c.Bind(&req) reads HTTP query body, parses json, fills struct req fields
	if err := c.Bind(&req); err != nil {
		// if json incorrect sending 400 Bad Request.
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// TODO: add field validation

	// business logic calls
	// sending data to UserService
	// c.Request().Context() to get context.Context from query
	newUser, err := h.service.RegisterUser(c.Request().Context(), req.Username, req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidUserInput) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		// if service returned error sending 500 Internal Server Error.
		// TODO: add error types for more clarity
		log.Printf("Error registering user: %v", err) // Logging error
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not register user"})
	}

	// Success respond 201 Created
	return c.JSON(http.StatusCreated, newUser)
}

// LoginRequest - структура для парсинга JSON-запроса на логин.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login - обработчик для роута POST /login.
func (h *Handler) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Вызываем сервис для проверки логина и пароля.
	token, err := h.service.Login(c.Request().Context(), req.Email, req.Password)
	if err != nil {
		// Если сервис вернул ошибку (неверные данные), отправляем 401 Unauthorized.
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
	}

	// Если все успешно, возвращаем токен клиенту.
	return c.JSON(http.StatusOK, map[string]string{
		"token": token,
	})
}
