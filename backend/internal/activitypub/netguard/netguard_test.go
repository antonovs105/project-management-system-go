package netguard

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRemoteURLRejectsUnsafeHosts(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1/users/alice",
		"http://[::1]/users/alice",
		"http://10.0.0.5/users/alice",
		"http://172.16.0.1/users/alice",
		"http://192.168.1.2/users/alice",
		"http://169.254.169.254/latest/meta-data",
		"http://100.64.0.1/users/alice",
		"http://192.0.2.1/users/alice",
		"http://[fe80::1]/users/alice",
		"ftp://remote.example/users/alice",
		"http://[fe80::1%25lo0]/users/alice",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := ValidateRemoteURL(raw)
			require.ErrorIs(t, err, ErrUnsafeURL)
		})
	}
}

func TestValidateRemoteURLAllowsPublicHTTPHosts(t *testing.T) {
	for _, raw := range []string{
		"https://remote.example/users/alice",
		"http://93.184.216.34/users/alice",
		"https://[2606:2800:220:1:248:1893:25c8:1946]/users/alice",
	} {
		t.Run(raw, func(t *testing.T) {
			parsed, err := ValidateRemoteURL(raw)
			require.NoError(t, err)
			assert.NotEmpty(t, parsed.Host)
		})
	}
}

func TestValidateRemoteURLRejectsHTTPWhenHTTPSRequired(t *testing.T) {
	_, err := ValidateRemoteURL("http://93.184.216.34/users/alice", RequireHTTPS())

	require.ErrorIs(t, err, ErrUnsafeURL)
	require.Contains(t, err.Error(), "https required")
}

func TestHTTPClientRejectsUnsafeRedirect(t *testing.T) {
	client := NewHTTPClient(time.Second)
	err := client.CheckRedirect(
		mustRequest(t, "http://127.0.0.1/private"),
		[]*http.Request{mustRequest(t, "https://remote.example/start")},
	)

	require.ErrorIs(t, err, ErrUnsafeURL)
}

func TestHTTPClientRejectsHTTPRedirectWhenHTTPSRequired(t *testing.T) {
	client := NewHTTPClientWithPolicy(time.Second, RequireHTTPS())
	err := client.CheckRedirect(
		mustRequest(t, "http://93.184.216.34/users/alice"),
		[]*http.Request{mustRequest(t, "https://remote.example/start")},
	)

	require.ErrorIs(t, err, ErrUnsafeURL)
	require.Contains(t, err.Error(), "https required")
}

func TestHTTPClientRejectsTooManyRedirects(t *testing.T) {
	client := NewHTTPClient(time.Second)
	via := []*http.Request{
		mustRequest(t, "https://remote.example/1"),
		mustRequest(t, "https://remote.example/2"),
		mustRequest(t, "https://remote.example/3"),
		mustRequest(t, "https://remote.example/4"),
		mustRequest(t, "https://remote.example/5"),
	}

	err := client.CheckRedirect(mustRequest(t, "https://remote.example/6"), via)

	require.True(t, errors.Is(err, ErrTooManyRedirects))
}

func mustRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	require.NoError(t, err)
	return req
}
