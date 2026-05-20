package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestValidateRuntimeConfigAllowsDevelopmentLocalhost(t *testing.T) {
	err := validateRuntimeConfig(false, "your_secret_key_here", "http://localhost:8080", "localhost:8080")

	require.NoError(t, err)
}

func TestValidateRuntimeConfigRejectsProductionDefaults(t *testing.T) {
	err := validateRuntimeConfig(true, "your_secret_key_here", "http://localhost:8080", "localhost:8080")

	require.Error(t, err)
}

func TestValidateRuntimeConfigAcceptsProductionValues(t *testing.T) {
	err := validateRuntimeConfig(true, "0123456789abcdef0123456789abcdef", "https://pm.example.test", "pm.example.test")

	require.NoError(t, err)
}

func TestGlobalMiddlewareAddsRequestIDAndStructuredLog(t *testing.T) {
	var logs bytes.Buffer
	e := echo.New()
	registerGlobalMiddleware(e, &logs)
	e.GET("/ping", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping?check=true", nil)
	req.Host = "api.example.test"
	req.Header.Set(echo.HeaderXRequestID, "req-123")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, "req-123", rec.Header().Get(echo.HeaderXRequestID))

	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry))
	require.Equal(t, "req-123", entry["request_id"])
	require.Equal(t, "GET", entry["method"])
	require.Equal(t, "/ping?check=true", entry["uri"])
	require.Equal(t, "/ping", entry["route"])
	require.Equal(t, float64(http.StatusNoContent), entry["status"])
}
