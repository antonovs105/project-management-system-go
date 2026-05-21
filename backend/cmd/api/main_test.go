package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/observability"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestValidateRuntimeConfigAllowsDevelopmentLocalhost(t *testing.T) {
	err := validateRuntimeConfig(false, "your_secret_key_here", "http://localhost:8080", "localhost:8080", "")

	require.NoError(t, err)
}

func TestParseAppRoleDefaultsToAll(t *testing.T) {
	role, err := parseAppRole("")

	require.NoError(t, err)
	require.Equal(t, appRoleAll, role)
	require.True(t, role.runsAPI())
	require.True(t, role.runsWorker())
}

func TestParseAppRoleSupportsSplitRoles(t *testing.T) {
	apiRole, err := parseAppRole("api")
	require.NoError(t, err)
	require.True(t, apiRole.runsAPI())
	require.False(t, apiRole.runsWorker())

	workerRole, err := parseAppRole(" worker ")
	require.NoError(t, err)
	require.False(t, workerRole.runsAPI())
	require.True(t, workerRole.runsWorker())
}

func TestParseAppRoleRejectsUnknownRole(t *testing.T) {
	_, err := parseAppRole("scheduler")

	require.Error(t, err)
	require.Contains(t, err.Error(), "APP_ROLE")
}

func TestValidateRuntimeConfigRejectsProductionDefaults(t *testing.T) {
	err := validateRuntimeConfig(true, "your_secret_key_here", "http://localhost:8080", "localhost:8080", "")

	require.Error(t, err)
}

func TestValidateRuntimeConfigAcceptsProductionValues(t *testing.T) {
	err := validateRuntimeConfig(true, "0123456789abcdef0123456789abcdef", "https://pm.example.test", "pm.example.test", "0123456789abcdef0123456789abcdef")

	require.NoError(t, err)
}

func TestValidateRuntimeConfigRejectsWeakProductionAdminBootstrapToken(t *testing.T) {
	err := validateRuntimeConfig(true, "0123456789abcdef0123456789abcdef", "https://pm.example.test", "pm.example.test", "short")

	require.Error(t, err)
	require.Contains(t, err.Error(), "ADMIN_BOOTSTRAP_TOKEN")
}

func TestRequiredDatabaseTablesIncludeActivityPubFoundation(t *testing.T) {
	require.Contains(t, requiredDatabaseTables, "actors")
	require.Contains(t, requiredDatabaseTables, "ap_activities")
	require.Contains(t, requiredDatabaseTables, "actor_inbox_items")
	require.Contains(t, requiredDatabaseTables, "actor_outbox_items")
	require.Contains(t, requiredDatabaseTables, "activity_deliveries")
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

func TestGlobalMiddlewareRejectsOversizedBody(t *testing.T) {
	e := echo.New()
	registerGlobalMiddleware(e, nil)
	e.POST("/upload", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("x"))
	req.ContentLength = defaultRequestBodyLimitBytes + 1
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestRateLimiterRejectsBurstExcess(t *testing.T) {
	e := echo.New()
	e.GET("/limited", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}, newRateLimiter(1, 1))

	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/limited", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestMetricsEndpointReportsHTTPRequests(t *testing.T) {
	metrics := observability.NewMetrics()
	server := &ApiServer{metrics: metrics}
	e := echo.New()
	registerGlobalMiddleware(e, nil, metrics)
	e.GET("/ping", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})
	e.GET("/metrics", server.metricsHandler)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get(echo.HeaderContentType), "text/plain")
	require.Contains(t, rec.Body.String(), "go_goroutines")
	require.Contains(t, rec.Body.String(), `pms_http_requests_total{method="GET",route="/ping",status="204"} 1`)
}
