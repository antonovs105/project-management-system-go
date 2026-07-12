package authsession

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewCookieUsesHardenedBrowserDefaults(t *testing.T) {
	cookie := NewCookie("signed-token", true)

	require.Equal(t, CookieName, cookie.Name)
	require.Equal(t, "/", cookie.Path)
	require.True(t, cookie.HttpOnly)
	require.True(t, cookie.Secure)
	require.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
	require.Equal(t, int((12 * time.Hour).Seconds()), cookie.MaxAge)
}

func TestClearCookieExpiresSession(t *testing.T) {
	cookie := ClearCookie(true)

	require.Equal(t, -1, cookie.MaxAge)
	require.True(t, cookie.Expires.Before(time.Now()))
}

func TestTokenFromRequestReadsOnlyNamedCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "other", Value: "ignored"})
	_, ok := TokenFromRequest(req)
	require.False(t, ok)

	req.AddCookie(&http.Cookie{Name: CookieName, Value: "signed-token"})
	token, ok := TokenFromRequest(req)
	require.True(t, ok)
	require.Equal(t, "signed-token", token)
}
