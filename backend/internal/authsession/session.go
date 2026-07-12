// Package authsession defines the browser session contract shared by the
// authentication service and HTTP middleware.
package authsession

import (
	"net/http"
	"time"
)

const (
	// CookieName is the host-scoped browser session cookie.
	CookieName = "progo.session"
	// Issuer identifies JWTs created by this application.
	Issuer = "progo"
	// Audience identifies JWTs intended for the Progo web application.
	Audience = "progo-web"
	// Lifetime bounds an authenticated browser session.
	Lifetime = 12 * time.Hour
)

// NewCookie returns a hardened browser session cookie.
func NewCookie(token string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(Lifetime),
		MaxAge:   int(Lifetime.Seconds()),
	}
}

// ClearCookie expires the browser session cookie immediately.
func ClearCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	}
}

// TokenFromRequest returns the session JWT carried by the browser cookie.
func TokenFromRequest(req *http.Request) (string, bool) {
	if req == nil {
		return "", false
	}
	cookie, err := req.Cookie(CookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}
