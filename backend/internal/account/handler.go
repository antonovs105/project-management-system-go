package account

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// Handler exposes account recovery and security lifecycle endpoints.
type Handler struct {
	service *Service
}

// NewHandler returns an account security HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterPublicRoutes mounts enumeration-resistant account challenge endpoints.
func (h *Handler) RegisterPublicRoutes(e *echo.Echo, middleware ...echo.MiddlewareFunc) {
	e.POST("/auth/password/forgot", h.ForgotPassword, middleware...)
	e.POST("/auth/password/reset", h.ResetPassword, middleware...)
	e.POST("/auth/email/verify", h.VerifyEmail, middleware...)
}

// RegisterAccountRoutes mounts authenticated account security endpoints.
func (h *Handler) RegisterAccountRoutes(api *echo.Group) {
	api.POST("/me/email/verification", h.RequestVerification)
	api.GET("/me/sessions", h.ListSessions)
	api.DELETE("/me/sessions/:sessionID", h.RevokeSession)
	api.GET("/me/security-events", h.ListSecurityEvents)
}

// ForgotPasswordRequest is the public recovery request payload.
type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

// ResetPasswordRequest is the single-use password replacement payload.
type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// VerifyEmailRequest is the single-use email ownership payload.
type VerifyEmailRequest struct {
	Token string `json:"token"`
}

// ForgotPassword accepts every syntactically valid request without revealing account existence.
func (h *Handler) ForgotPassword(c echo.Context) error {
	var request ForgotPasswordRequest
	if err := c.Bind(&request); err == nil {
		if err := h.service.RequestPasswordReset(c.Request().Context(), request.Email, clientInfo(c)); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "recovery request failed"})
		}
	}
	return c.NoContent(http.StatusAccepted)
}

// ResetPassword consumes a recovery challenge and revokes every existing session.
func (h *Handler) ResetPassword(c echo.Context) error {
	var request ResetPasswordRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := h.service.ResetPassword(c.Request().Context(), request.Token, request.NewPassword, clientInfo(c)); err != nil {
		if errors.Is(err, ErrInvalidToken) || errors.Is(err, ErrInvalidPassword) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "password reset failed"})
	}
	return c.NoContent(http.StatusNoContent)
}

// VerifyEmail consumes an email-ownership challenge.
func (h *Handler) VerifyEmail(c echo.Context) error {
	var request VerifyEmailRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := h.service.VerifyEmail(c.Request().Context(), request.Token); err != nil {
		if errors.Is(err, ErrInvalidToken) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "email verification failed"})
	}
	return c.NoContent(http.StatusNoContent)
}

// RequestVerification queues a replacement email for the current account.
func (h *Handler) RequestVerification(c echo.Context) error {
	if err := h.service.RequestVerification(c.Request().Context(), currentUserID(c)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "verification request failed"})
	}
	return c.NoContent(http.StatusAccepted)
}

// ListSessions returns recent active and revoked browser sessions.
func (h *Handler) ListSessions(c echo.Context) error {
	values, err := h.service.ListSessions(c.Request().Context(), currentUserID(c), currentSessionID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "session list failed"})
	}
	return c.JSON(http.StatusOK, values)
}

// RevokeSession invalidates one session owned by the current account.
func (h *Handler) RevokeSession(c echo.Context) error {
	if err := h.service.RevokeSession(c.Request().Context(), currentUserID(c), c.Param("sessionID"), clientInfo(c)); err != nil {
		if errors.Is(err, ErrInvalidToken) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "session not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "session revoke failed"})
	}
	return c.NoContent(http.StatusNoContent)
}

// ListSecurityEvents returns the account's durable authentication history.
func (h *Handler) ListSecurityEvents(c echo.Context) error {
	values, err := h.service.ListAuthEvents(c.Request().Context(), currentUserID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "security event list failed"})
	}
	return c.JSON(http.StatusOK, values)
}

// currentUserID reads the authenticated local user identifier.
func currentUserID(c echo.Context) string {
	value, _ := c.Get("userID").(string)
	return strings.TrimSpace(value)
}

// currentSessionID reads the validated browser session identifier.
func currentSessionID(c echo.Context) string {
	value, _ := c.Get("sessionID").(string)
	return strings.TrimSpace(value)
}

// clientInfo captures proxy-normalized request metadata.
func clientInfo(c echo.Context) ClientInfo {
	return ClientInfo{IPAddress: c.RealIP(), UserAgent: strings.TrimSpace(c.Request().UserAgent())}
}
