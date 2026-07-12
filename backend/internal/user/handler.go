package user

import (
	"crypto/hmac"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/account"
	"github.com/antonovs105/project-management-system-go/internal/apiresponse"
	"github.com/antonovs105/project-management-system-go/internal/authsession"
	"github.com/labstack/echo/v4"
)

// oauthStateCookieName stores the browser-bound verifier for OAuth callbacks.
const oauthStateCookieName = "progo.oauth_state"

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
	e.GET("/auth/oauth/providers", h.OAuthProviders, middleware...)
	e.GET("/auth/:provider/start", h.StartOAuth, middleware...)
	e.GET("/auth/:provider/callback", h.OAuthCallback, middleware...)
	e.POST("/auth/oauth/exchange", h.ExchangeOAuthCode, middleware...)
}

// RegisterAdminRoutes registers authenticated admin-only user routes.
func (h *Handler) RegisterAdminRoutes(api *echo.Group) {
	api.GET("/admin/users", h.ListUsers)
	api.PATCH("/admin/users/:userID/role", h.UpdateInstanceRole)
}

// RegisterAccountRoutes registers authenticated self-service account routes.
func (h *Handler) RegisterAccountRoutes(api *echo.Group) {
	api.PATCH("/me/password", h.ChangePassword)
	api.POST("/me/logout", h.Logout)
}

// RegisterRequest is the JSON payload for local account registration.
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// ExchangeOAuthCodeRequest is the JSON payload for completing OAuth login in the SPA.
type ExchangeOAuthCodeRequest struct {
	Code    string `json:"code"`
	MFACode string `json:"mfa_code,omitempty"`
}

// SessionResponse is the non-sensitive identity returned after a session cookie is established.
type SessionResponse struct {
	UserID                string `json:"user_id"`
	InstanceRole          string `json:"instance_role"`
	Email                 string `json:"email,omitempty"`
	EmailVerified         bool   `json:"email_verified"`
	MFAEnrollmentRequired bool   `json:"mfa_enrollment_required,omitempty"`
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
	return apiresponse.WriteOffsetPage(c, http.StatusOK, users, options.Limit, options.Offset)
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
	case errors.Is(err, ErrCannotDemoteLastOwner):
		return c.JSON(http.StatusConflict, map[string]string{"error": ErrCannotDemoteLastOwner.Error()})
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
		if errors.Is(err, ErrRegistrationDisabled) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": ErrRegistrationDisabled.Error()})
		}
		if errors.Is(err, ErrInvalidUserInput) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		log.Printf("Error registering user: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not register user"})
	}

	return c.JSON(http.StatusCreated, newUser)
}

// OAuthProviders returns currently configured optional OAuth providers.
func (h *Handler) OAuthProviders(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string][]string{"providers": h.service.EnabledOAuthProviders()})
}

// StartOAuth redirects the browser to the requested provider authorization screen.
func (h *Handler) StartOAuth(c echo.Context) error {
	authURL, err := h.service.OAuthStartURL(c.Param("provider"))
	if err != nil {
		return writeOAuthStartError(c, err)
	}
	state, err := oauthStateFromAuthURL(authURL)
	if err != nil {
		return writeOAuthStartError(c, err)
	}
	setOAuthStateCookie(c, state, h.service.oauthConfig.StateTTL)
	return c.Redirect(http.StatusFound, authURL)
}

// OAuthCallback receives the provider callback and redirects back to the SPA with a one-time code.
func (h *Handler) OAuthCallback(c echo.Context) error {
	provider := c.Param("provider")
	if providerError := strings.TrimSpace(c.QueryParam("error")); providerError != "" {
		clearOAuthStateCookie(c)
		return c.Redirect(http.StatusFound, h.service.OAuthFrontendRedirectURL("", "provider_denied"))
	}
	if !validOAuthStateCookie(c, c.QueryParam("state")) {
		clearOAuthStateCookie(c)
		return c.Redirect(http.StatusFound, h.service.OAuthFrontendRedirectURL("", "invalid_state"))
	}
	clearOAuthStateCookie(c)
	code, err := h.service.CompleteOAuthLogin(c.Request().Context(), provider, c.QueryParam("code"), c.QueryParam("state"))
	if err != nil {
		return c.Redirect(http.StatusFound, h.service.OAuthFrontendRedirectURL("", oauthErrorCode(err)))
	}
	return c.Redirect(http.StatusFound, h.service.OAuthFrontendRedirectURL(code, ""))
}

// ExchangeOAuthCode consumes a one-time OAuth code and returns a normal bearer JWT.
func (h *Handler) ExchangeOAuthCode(c echo.Context) error {
	var req ExchangeOAuthCodeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	session, err := h.service.ExchangeOAuthSessionWithFactor(c.Request().Context(), req.Code, req.MFACode, requestClientInfo(c))
	if err != nil {
		return writeOAuthExchangeError(c, err)
	}
	setSessionCookie(c, session.Token)
	return c.JSON(http.StatusOK, sessionResponse(session))
}

// LoginRequest is the JSON payload for password login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	MFACode  string `json:"mfa_code,omitempty"`
}

// Login verifies credentials and returns a bearer JWT.
func (h *Handler) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	session, err := h.service.LoginSessionWithFactor(c.Request().Context(), req.Email, req.Password, req.MFACode, requestClientInfo(c))
	if err != nil {
		if errors.Is(err, ErrEmailNotVerified) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
		}
		if errors.Is(err, account.ErrMFARequired) || errors.Is(err, account.ErrMFAInvalid) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error(), "code": "mfa_required"})
		}
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
	}
	setSessionCookie(c, session.Token)

	return c.JSON(http.StatusOK, sessionResponse(session))
}

// Logout revokes the user's issued sessions and expires the browser cookie.
func (h *Handler) Logout(c echo.Context) error {
	if err := h.service.RevokeSession(c.Request().Context(), currentUserID(c), currentSessionID(c), requestClientInfo(c)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "logout failed"})
	}
	c.SetCookie(authsession.ClearCookie(oauthCookieSecure(c)))
	return c.NoContent(http.StatusNoContent)
}

// writeOAuthStartError maps OAuth authorization-start failures to public responses.
func writeOAuthStartError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrOAuthProviderUnavailable):
		return c.JSON(http.StatusNotFound, map[string]string{"error": ErrOAuthProviderUnavailable.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "oauth start failed"})
	}
}

// writeOAuthExchangeError maps one-time OAuth code failures to public responses.
func writeOAuthExchangeError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrOAuthInvalidCode):
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": ErrOAuthInvalidCode.Error()})
	case errors.Is(err, account.ErrMFARequired), errors.Is(err, account.ErrMFAInvalid):
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error(), "code": "mfa_required"})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "oauth exchange failed"})
	}
}

// oauthErrorCode converts OAuth service errors into safe SPA callback codes.
func oauthErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrOAuthProviderUnavailable):
		return "provider_unavailable"
	case errors.Is(err, ErrOAuthInvalidState):
		return "invalid_state"
	case errors.Is(err, ErrOAuthEmailNotVerified):
		return "email_unverified"
	case errors.Is(err, ErrOAuthEmailAlreadyRegistered):
		return "email_registered"
	case errors.Is(err, ErrRegistrationDisabled):
		return "registration_disabled"
	default:
		return "provider_failed"
	}
}

// oauthStateFromAuthURL extracts the signed state generated for the provider redirect.
func oauthStateFromAuthURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	state := strings.TrimSpace(parsed.Query().Get("state"))
	if state == "" {
		return "", ErrOAuthInvalidState
	}
	return state, nil
}

// setOAuthStateCookie stores a callback verifier tied to this browser flow.
func setOAuthStateCookie(c echo.Context, signedState string, ttl time.Duration) {
	c.SetCookie(&http.Cookie{
		Name:     oauthStateCookieName,
		Value:    hashOAuthCode(signedState),
		Path:     "/auth",
		HttpOnly: true,
		Secure:   oauthCookieSecure(c),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(ttl),
		MaxAge:   int(ttl.Seconds()),
	})
}

// validOAuthStateCookie reports whether callback state matches the browser verifier.
func validOAuthStateCookie(c echo.Context, signedState string) bool {
	signedState = strings.TrimSpace(signedState)
	if signedState == "" {
		return false
	}
	cookie, err := c.Cookie(oauthStateCookieName)
	if err != nil {
		return false
	}
	expected := hashOAuthCode(signedState)
	return hmac.Equal([]byte(cookie.Value), []byte(expected))
}

// clearOAuthStateCookie removes the browser verifier after any callback outcome.
func clearOAuthStateCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		Path:     "/auth",
		HttpOnly: true,
		Secure:   oauthCookieSecure(c),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

// oauthCookieSecure detects HTTPS directly or through a reverse proxy.
func oauthCookieSecure(c echo.Context) bool {
	return c.IsTLS() || strings.EqualFold(c.Request().Header.Get("X-Forwarded-Proto"), "https")
}

// setSessionCookie stores the authenticated session outside JavaScript access.
func setSessionCookie(c echo.Context, token string) {
	c.SetCookie(authsession.NewCookie(token, oauthCookieSecure(c)))
}

// sessionResponse maps the authenticated user to the browser-safe response.
func sessionResponse(session *AuthenticatedSession) SessionResponse {
	if session == nil || session.User == nil {
		return SessionResponse{}
	}
	value := session.User
	return SessionResponse{UserID: value.ID, InstanceRole: value.InstanceRole, Email: value.Email, EmailVerified: value.EmailVerified, MFAEnrollmentRequired: session.MFAEnrollmentRequired}
}

// currentSessionID returns the validated browser session claim.
func currentSessionID(c echo.Context) string {
	value, _ := c.Get("sessionID").(string)
	return strings.TrimSpace(value)
}

// requestClientInfo captures proxy-normalized request metadata for session history.
func requestClientInfo(c echo.Context) account.ClientInfo {
	return account.ClientInfo{IPAddress: c.RealIP(), UserAgent: strings.TrimSpace(c.Request().UserAgent())}
}
