package user

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// Handler exposes user registration, login, account, and admin HTTP endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates a user HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}

// RegisterRoutes registers public user routes on the root Echo router.
func (h *Handler) RegisterRoutes(e *echo.Echo, middleware ...echo.MiddlewareFunc) {
	e.POST("/register", h.Register, middleware...)
	e.POST("/login", h.Login, middleware...)
}

// RegisterAdminRoutes registers authenticated admin-only user routes.
func (h *Handler) RegisterAdminRoutes(api *echo.Group) {
	api.GET("/admin/users", h.ListUsers)
	api.PATCH("/admin/users/:userID/role", h.UpdateInstanceRole)
}

// RegisterAccountRoutes registers authenticated self-service account routes.
func (h *Handler) RegisterAccountRoutes(api *echo.Group) {
	api.PATCH("/me/password", h.ChangePassword)
}

// RegisterRequest is the JSON payload for local account registration.
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UpdateInstanceRoleRequest is the JSON payload for changing a user's instance role.
type UpdateInstanceRoleRequest struct {
	InstanceRole string `json:"instance_role"`
	Role         string `json:"role,omitempty"`
}

// ChangePasswordRequest is the JSON payload for replacing the current user's password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ListUsers returns an admin-filtered list of local users.
func (h *Handler) ListUsers(c echo.Context) error {
	options, err := listUsersOptions(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	users, err := h.service.ListUsers(c.Request().Context(), currentUserID(c), options)
	if err != nil {
		return writeAdminUserError(c, err)
	}
	return c.JSON(http.StatusOK, users)
}

// UpdateInstanceRole changes a user's instance role.
func (h *Handler) UpdateInstanceRole(c echo.Context) error {
	var req UpdateInstanceRoleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	role := req.InstanceRole
	if role == "" {
		role = req.Role
	}
	updated, err := h.service.UpdateInstanceRole(c.Request().Context(), currentUserID(c), c.Param("userID"), role)
	if err != nil {
		return writeAdminUserError(c, err)
	}
	return c.JSON(http.StatusOK, updated)
}

// ChangePassword replaces the authenticated user's password.
func (h *Handler) ChangePassword(c echo.Context) error {
	var req ChangePasswordRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	err := h.service.ChangePassword(c.Request().Context(), currentUserID(c), req.CurrentPassword, req.NewPassword)
	if err != nil {
		return writeAccountError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// currentUserID returns the authenticated user ID stored by JWT middleware.
func currentUserID(c echo.Context) string {
	userID, _ := c.Get("userID").(string)
	return userID
}

// listUsersOptions parses admin user list filters from query parameters.
func listUsersOptions(c echo.Context) (ListUsersOptions, error) {
	limit, err := parseOptionalLimit(c.QueryParam("limit"))
	if err != nil {
		return ListUsersOptions{}, ErrInvalidUserInput
	}
	offset, err := parseOptionalOffset(c.QueryParam("offset"))
	if err != nil {
		return ListUsersOptions{}, ErrInvalidUserInput
	}
	return ListUsersOptions{
		InstanceRole: strings.TrimSpace(c.QueryParam("role")),
		Query:        strings.TrimSpace(c.QueryParam("q")),
		Limit:        limit,
		Offset:       offset,
	}, nil
}

// parseOptionalLimit parses an optional positive pagination limit.
func parseOptionalLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, ErrInvalidUserInput
	}
	return value, nil
}

// parseOptionalOffset parses an optional non-negative pagination offset.
func parseOptionalOffset(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, ErrInvalidUserInput
	}
	return value, nil
}

// writeAdminUserError maps admin user service errors to stable HTTP responses.
func writeAdminUserError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrAdminRequired):
		return c.JSON(http.StatusForbidden, map[string]string{"error": ErrAdminRequired.Error()})
	case errors.Is(err, ErrOwnerRequired):
		return c.JSON(http.StatusForbidden, map[string]string{"error": ErrOwnerRequired.Error()})
	case errors.Is(err, ErrInvalidUserInput):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrUserNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": ErrUserNotFound.Error()})
	case errors.Is(err, ErrCannotDemoteLastAdmin):
		return c.JSON(http.StatusConflict, map[string]string{"error": ErrCannotDemoteLastAdmin.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "admin user operation failed"})
	}
}

// writeAccountError maps account service errors to stable HTTP responses.
func writeAccountError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrInvalidUserInput):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrInvalidCredentials):
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": ErrInvalidCredentials.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "account operation failed"})
	}
}

// Register creates a regular account from the public registration endpoint.
func (h *Handler) Register(c echo.Context) error {
	var req RegisterRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	newUser, err := h.service.RegisterUser(c.Request().Context(), req.Username, req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidUserInput) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		log.Printf("Error registering user: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not register user"})
	}

	return c.JSON(http.StatusCreated, newUser)
}

// LoginRequest is the JSON payload for password login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login verifies credentials and returns a bearer JWT.
func (h *Handler) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	token, err := h.service.Login(c.Request().Context(), req.Email, req.Password)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"token": token,
	})
}
