package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
)

func TestServerReadinessAndMetrics(t *testing.T) {
	t.Parallel()

	server := NewServer(config.ObservabilityConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
		ReadyPath:     "/readyz",
	}, NewMetrics())

	handler := server.server.Handler

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("expected health status 200, got %d", health.Code)
	}

	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected readiness status 503 before ready, got %d", ready.Code)
	}

	server.SetReady(true)

	ready = httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("expected readiness status 200 after ready, got %d", ready.Code)
	}

	metrics := httptest.NewRecorder()
	handler.ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metrics.Code != http.StatusOK {
		t.Fatalf("expected metrics status 200, got %d", metrics.Code)
	}
}
