// Package observability owns process metrics exported for monitoring systems.
package observability

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics contains the backend Prometheus registry and application collectors.
type Metrics struct {
	registry             *prometheus.Registry
	httpRequestsTotal    *prometheus.CounterVec
	httpRequestDuration  *prometheus.HistogramVec
	httpRequestsInFlight prometheus.Gauge
}

// NewMetrics creates a Prometheus registry with Go, process, and HTTP collectors.
func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	metrics := &Metrics{
		registry: registry,
		httpRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pms",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total HTTP requests handled by method, route, and status.",
		}, []string{"method", "route", "status"}),
		httpRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pms",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds by method, route, and status.",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"method", "route", "status"}),
		httpRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "pms",
			Subsystem: "http",
			Name:      "requests_in_flight",
			Help:      "Current number of HTTP requests being handled.",
		}),
	}

	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		metrics.httpRequestsTotal,
		metrics.httpRequestDuration,
		metrics.httpRequestsInFlight,
	)
	return metrics
}

// Handler returns a Prometheus HTTP handler for this registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// IncHTTPInFlight increments the in-flight HTTP request gauge.
func (m *Metrics) IncHTTPInFlight() {
	if m == nil {
		return
	}
	m.httpRequestsInFlight.Inc()
}

// DecHTTPInFlight decrements the in-flight HTTP request gauge.
func (m *Metrics) DecHTTPInFlight() {
	if m == nil {
		return
	}
	m.httpRequestsInFlight.Dec()
}

// ObserveHTTPRequest records one completed HTTP request.
func (m *Metrics) ObserveHTTPRequest(method, route string, status int, duration time.Duration) {
	if m == nil {
		return
	}
	method = normalizeMethod(method)
	route = normalizeRoute(route)
	statusLabel := strconv.Itoa(normalizeStatus(status))
	seconds := duration.Seconds()
	if seconds < 0 {
		seconds = 0
	}

	m.httpRequestsTotal.WithLabelValues(method, route, statusLabel).Inc()
	m.httpRequestDuration.WithLabelValues(method, route, statusLabel).Observe(seconds)
}

// normalizeMethod returns a bounded HTTP method label.
func normalizeMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return "UNKNOWN"
	}
	return method
}

// normalizeRoute returns a bounded route label for unmatched requests.
func normalizeRoute(route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		return "unknown"
	}
	return route
}

// normalizeStatus returns a valid HTTP status label.
func normalizeStatus(status int) int {
	if status <= 0 {
		return http.StatusInternalServerError
	}
	return status
}
