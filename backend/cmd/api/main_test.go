package main

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/account"
	"github.com/antonovs105/project-management-system-go/internal/activityhistory"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	apfederation "github.com/antonovs105/project-management-system-go/internal/activitypub/federation"
	apmoderation "github.com/antonovs105/project-management-system-go/internal/activitypub/moderation"
	"github.com/antonovs105/project-management-system-go/internal/adminaudit"
	"github.com/antonovs105/project-management-system-go/internal/attachment"
	"github.com/antonovs105/project-management-system-go/internal/comment"
	appconfig "github.com/antonovs105/project-management-system-go/internal/config"
	"github.com/antonovs105/project-management-system-go/internal/githubintegration"
	"github.com/antonovs105/project-management-system-go/internal/instance"
	"github.com/antonovs105/project-management-system-go/internal/label"
	"github.com/antonovs105/project-management-system-go/internal/notification"
	"github.com/antonovs105/project-management-system-go/internal/observability"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type failingReadinessDriver struct{}

func (failingReadinessDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("database unavailable")
}

func TestValidateRuntimeConfigAllowsDevelopmentLocalhost(t *testing.T) {
	err := validateRuntimeConfig(false, "your_secret_key_here", "http://localhost:8080", "localhost:8080", "", "")

	require.NoError(t, err)
}

func TestHealthCheckIsIndependentOfExternalDependencies(t *testing.T) {
	server := &ApiServer{}
	e := echo.New()
	e.GET("/health", server.healthCheck)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"status":"ok","system":"alive"}`, rec.Body.String())
}

func TestReadinessCheckFailsClosedWhenDatabaseIsUnavailable(t *testing.T) {
	const driverName = "progo-readiness-failure"
	sql.Register(driverName, failingReadinessDriver{})
	db := sqlx.NewDb(sql.OpenDB(failingReadinessDriverConnector{}), driverName)
	defer db.Close()

	server := &ApiServer{db: db}
	e := echo.New()
	e.GET("/ready", server.readinessCheck)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.JSONEq(t, `{"status":"not_ready","checks":{"database":"error","redis":"disabled"}}`, rec.Body.String())
}

type failingReadinessDriverConnector struct{}

func (failingReadinessDriverConnector) Connect(context.Context) (driver.Conn, error) {
	return nil, errors.New("database unavailable")
}

func (failingReadinessDriverConnector) Driver() driver.Driver {
	return failingReadinessDriver{}
}

func TestAuthAccountRateLimitIdentifierHashesNormalizedEmailAndPreservesBody(t *testing.T) {
	e := echo.New()
	identifierFor := func(email, ip string) (string, string) {
		body := `{"email":"` + email + `","password":"secret"}`
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
		req.RemoteAddr = ip + ":1234"
		ctx := e.NewContext(req, httptest.NewRecorder())
		ctx.SetPath("/login")

		identifier, err := authAccountRateLimitIdentifier(ctx)
		require.NoError(t, err)
		preserved, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		return identifier, string(preserved)
	}

	first, body := identifierFor(" User@Example.Test ", "192.0.2.10")
	second, _ := identifierFor("user@example.test", "198.51.100.20")

	require.Equal(t, first, second)
	require.NotContains(t, first, "example.test")
	require.JSONEq(t, `{"email":" User@Example.Test ","password":"secret"}`, body)
}

func TestParseAppEnvDefaultsToDevelopment(t *testing.T) {
	env, err := parseAppEnv("")

	require.NoError(t, err)
	require.Equal(t, appEnvDevelopment, env)
}

func TestParseAppEnvRejectsUnknownValue(t *testing.T) {
	_, err := parseAppEnv("prod")

	require.Error(t, err)
	require.Contains(t, err.Error(), "APP_ENV")
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
	err := validateRuntimeConfig(true, "your_secret_key_here", "http://localhost:8080", "localhost:8080", "", "")

	require.Error(t, err)
}

func TestValidateRuntimeConfigAcceptsProductionValues(t *testing.T) {
	err := validateRuntimeConfig(true, "0123456789abcdef0123456789abcdef", "https://pm.example.test", "pm.example.test", "metrics-token-0123456789abcdef0123456789", "actor-key-0123456789abcdef0123456789")

	require.NoError(t, err)
}

func TestValidateRuntimeConfigRejectsMissingProductionMetricsToken(t *testing.T) {
	err := validateRuntimeConfig(true, "0123456789abcdef0123456789abcdef", "https://pm.example.test", "pm.example.test", "", "actor-key-0123456789abcdef0123456789")

	require.Error(t, err)
	require.Contains(t, err.Error(), "METRICS_TOKEN")
}

func TestValidateRuntimeConfigRejectsMissingProductionActorPrivateKeyEncryptionKey(t *testing.T) {
	err := validateRuntimeConfig(true, "0123456789abcdef0123456789abcdef", "https://pm.example.test", "pm.example.test", "metrics-token-0123456789abcdef0123456789", "")

	require.Error(t, err)
	require.Contains(t, err.Error(), "ACTOR_PRIVATE_KEY_ENCRYPTION_KEY")
}

func TestOptionalBoolEnvParsesSupportedValues(t *testing.T) {
	t.Setenv("FEDERATION_ALLOW_INSECURE_HTTP", " yes ")

	value, ok, err := optionalBoolEnv("FEDERATION_ALLOW_INSECURE_HTTP")

	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, value)
}

func TestOptionalBoolEnvRejectsInvalidValues(t *testing.T) {
	t.Setenv("FEDERATION_ALLOW_INSECURE_HTTP", "sometimes")

	_, ok, err := optionalBoolEnv("FEDERATION_ALLOW_INSECURE_HTTP")

	require.Error(t, err)
	require.True(t, ok)
	require.Contains(t, err.Error(), "FEDERATION_ALLOW_INSECURE_HTTP")
}

func TestValidateCORSConfigAllowsDevelopmentDefaults(t *testing.T) {
	require.NoError(t, validateCORSConfig(false, nil))
	require.NoError(t, validateCORSConfig(false, []string{"*"}))
}

func TestValidateCORSConfigAcceptsProductionOrigins(t *testing.T) {
	err := validateCORSConfig(true, []string{"https://app.example.test", "https://admin.example.test:8443"})

	require.NoError(t, err)
}

func TestValidateCORSConfigRejectsUnsafeProductionOrigins(t *testing.T) {
	cases := []struct {
		name    string
		origins []string
	}{
		{name: "missing", origins: nil},
		{name: "wildcard", origins: []string{"*"}},
		{name: "localhost", origins: []string{"http://localhost:5173"}},
		{name: "with path", origins: []string{"https://app.example.test/dashboard"}},
		{name: "non http", origins: []string{"chrome-extension://extension-id"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCORSConfig(true, tc.origins)

			require.Error(t, err)
			require.Contains(t, err.Error(), "CORS_ALLOWED_ORIGINS")
		})
	}
}

func TestTrustedProxyIPExtractorDefaultsToDirectRemoteAddress(t *testing.T) {
	extractor, err := trustedProxyIPExtractor(nil)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "203.0.113.10:54321"
	req.Header.Set(echo.HeaderXForwardedFor, "198.51.100.20")

	require.Equal(t, "203.0.113.10", extractor(req))
}

func TestTrustedProxyIPExtractorUsesForwardedForFromTrustedProxy(t *testing.T) {
	extractor, err := trustedProxyIPExtractor([]string{"192.0.2.0/24"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.0.2.10:54321"
	req.Header.Set(echo.HeaderXForwardedFor, "198.51.100.20")

	require.Equal(t, "198.51.100.20", extractor(req))
}

func TestTrustedProxyIPExtractorRejectsInvalidCIDR(t *testing.T) {
	_, err := trustedProxyIPExtractor([]string{"192.0.2.10"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "TRUSTED_PROXY_CIDRS")
}

func TestRequiredDatabaseTablesIncludeActivityPubFoundation(t *testing.T) {
	require.Contains(t, requiredDatabaseTables, "actors")
	require.Contains(t, requiredDatabaseTables, "ap_activities")
	require.Contains(t, requiredDatabaseTables, "actor_inbox_items")
	require.Contains(t, requiredDatabaseTables, "actor_outbox_items")
	require.Contains(t, requiredDatabaseTables, "activity_deliveries")
	require.Contains(t, requiredDatabaseTables, "project_roles")
	require.Contains(t, requiredDatabaseTables, "project_role_permissions")
}

func TestAuthenticatedAPIRoutesUseVersionedPrefixOnly(t *testing.T) {
	e := echo.New()
	server := &ApiServer{
		userHandler:         user.NewHandler(nil),
		accountHandler:      account.NewHandler(nil),
		activityHandler:     activityhistory.NewHandler(nil),
		attachmentHandler:   attachment.NewHandler(nil),
		instanceHandler:     instance.NewHandler(appconfig.Config{}, nil, nil),
		projectHandler:      project.NewHandler(nil),
		labelHandler:        label.NewHandler(nil),
		ticketHandler:       ticket.NewHandler(nil),
		commentHandler:      comment.NewHandler(nil),
		notificationHandler: notification.NewHandler(nil),
		githubHandler:       githubintegration.NewHandler(nil),
		federationHandler:   apfederation.NewHandler(nil),
		moderationHandler:   apmoderation.NewHandler(nil),
		deliveryHandler:     delivery.NewHandler(nil),
		auditHandler:        adminaudit.NewHandler(nil),
	}

	registerAuthenticatedAPIRoutes(e.Group("/api/v1"), server, []byte("test-secret"), nil)

	paths := make(map[string]struct{})
	for _, route := range e.Routes() {
		paths[route.Path] = struct{}{}
		require.True(t, route.Path == "/api/v1" || strings.HasPrefix(route.Path, "/api/v1/"), "unexpected authenticated API route %s", route.Path)
		legacyRESTPath := strings.HasPrefix(route.Path, "/api/") && route.Path != "/api/v1" && !strings.HasPrefix(route.Path, "/api/v1/")
		require.False(t, legacyRESTPath, "legacy API route mounted: %s", route.Path)
	}
	require.Contains(t, paths, "/api/v1/projects")
	require.Contains(t, paths, "/api/v1/me/password")
	require.NotContains(t, paths, "/api/projects")
}

func TestAuthenticatedRuntimeRoutesExistInOpenAPI(t *testing.T) {
	e := echo.New()
	server := &ApiServer{
		userHandler:         user.NewHandler(nil),
		accountHandler:      account.NewHandler(nil),
		activityHandler:     activityhistory.NewHandler(nil),
		attachmentHandler:   attachment.NewHandler(nil),
		instanceHandler:     instance.NewHandler(appconfig.Config{}, nil, nil),
		projectHandler:      project.NewHandler(nil),
		labelHandler:        label.NewHandler(nil),
		ticketHandler:       ticket.NewHandler(nil),
		commentHandler:      comment.NewHandler(nil),
		notificationHandler: notification.NewHandler(nil),
		githubHandler:       githubintegration.NewHandler(nil),
		federationHandler:   apfederation.NewHandler(nil),
		moderationHandler:   apmoderation.NewHandler(nil),
		deliveryHandler:     delivery.NewHandler(nil),
		auditHandler:        adminaudit.NewHandler(nil),
	}
	registerAuthenticatedAPIRoutes(e.Group("/api/v1"), server, []byte("test-secret"), nil)

	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "openapi.yaml"))
	require.NoError(t, err)
	var document struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &document))
	documented := make(map[string]struct{})
	for path, operations := range document.Paths {
		for method := range operations {
			if isHTTPMethod(method) {
				documented[strings.ToUpper(method)+" "+normalizedRoutePath(path)] = struct{}{}
			}
		}
	}
	for _, route := range e.Routes() {
		if route.Path == "/api/v1" || !isHTTPMethod(route.Method) {
			continue
		}
		key := route.Method + " " + normalizedRoutePath(route.Path)
		require.Contains(t, documented, key, "runtime route is missing from OpenAPI: %s", key)
	}
}

func normalizedRoutePath(path string) string {
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		if strings.HasPrefix(segment, ":") || (strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")) {
			segments[index] = "{}"
		}
	}
	return strings.Join(segments, "/")
}

func isHTTPMethod(value string) bool {
	switch strings.ToUpper(value) {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
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

func TestMetricsEndpointRequiresBearerTokenWhenConfigured(t *testing.T) {
	metrics := observability.NewMetrics()
	server := &ApiServer{
		metrics:      metrics,
		metricsToken: "metrics-token-0123456789abcdef0123456789",
	}
	e := echo.New()
	e.GET("/metrics", server.metricsHandler)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer wrong-token")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer metrics-token-0123456789abcdef0123456789")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "go_goroutines")
}
