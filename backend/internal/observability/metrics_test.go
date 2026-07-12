package observability

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestMetricsHandlerExportsPrometheusCollectors(t *testing.T) {
	metrics := NewMetrics()
	metrics.ObserveHTTPRequest(http.MethodGet, "/health", http.StatusOK, 25*time.Millisecond)
	statusCode := http.StatusTooManyRequests
	metrics.ObserveDeliveryAttempt("retryable_failure", "http", &statusCode, 50*time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, req)

	body := rec.Body.String()
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	require.Contains(t, body, "go_goroutines")
	require.Contains(t, body, "process_cpu_seconds_total")
	require.Contains(t, body, `pms_http_requests_total{method="GET",route="/health",status="200"} 1`)
	require.Contains(t, body, "pms_http_request_duration_seconds_bucket")
	require.Contains(t, body, `pms_activitypub_delivery_attempts_total{failure_kind="http",outcome="retryable_failure",status_code="429"} 1`)
	require.Contains(t, body, "pms_activitypub_delivery_attempt_duration_seconds_bucket")
}

func TestMetricsExportsDatabasePoolStats(t *testing.T) {
	db, err := sql.Open("postgres", "postgres://unused")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(25)

	metrics := NewMetrics()
	metrics.RegisterDBStats(db, "primary")
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	require.Contains(t, rec.Body.String(), `go_sql_max_open_connections{db_name="primary"} 25`)
	require.Contains(t, rec.Body.String(), `go_sql_open_connections{db_name="primary"} 0`)
}

func TestMetricsNormalizesEmptyLabels(t *testing.T) {
	metrics := NewMetrics()
	metrics.ObserveHTTPRequest("", "", 0, -time.Second)
	metrics.ObserveDeliveryAttempt("", "", nil, -time.Second)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, req)

	require.Contains(t, rec.Body.String(), `pms_http_requests_total{method="UNKNOWN",route="unknown",status="500"} 1`)
	require.Contains(t, rec.Body.String(), `pms_activitypub_delivery_attempts_total{failure_kind="none",outcome="unknown",status_code="none"} 1`)
}
