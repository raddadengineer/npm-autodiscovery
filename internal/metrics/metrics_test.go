package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsRegistry(t *testing.T) {
	reg := NewRegistry("1.1.0")

	// Set active proxies
	reg.SetActiveProxies("core-01", "docker", 14)
	reg.SetActiveProxies("core-01", "iac", 3)

	// Increment events
	reg.IncEvents("start", "success")
	reg.IncEvents("start", "success")
	reg.IncEvents("die", "success")

	// Health transitions
	reg.IncHealthStatus("healthy")
	reg.IncHealthStatus("healthy")
	reg.IncHealthStatus("unhealthy")

	// Observe sync duration
	reg.ObserveSyncDuration(40 * time.Millisecond) // <= 0.05
	reg.ObserveSyncDuration(80 * time.Millisecond) // <= 0.1

	out := reg.FormatMetrics()

	// Verify Prometheus format requirements from ROADMAP.md
	expectedLines := []string{
		`npm_autodiscovery_build_info{version="1.1.0"} 1`,
		`# HELP npm_autodiscovery_active_proxies Total number of active proxy hosts managed`,
		`# TYPE npm_autodiscovery_active_proxies gauge`,
		`npm_autodiscovery_active_proxies{host_id="core-01",source="docker"} 14`,
		`npm_autodiscovery_active_proxies{host_id="core-01",source="iac"} 3`,
		`# HELP npm_autodiscovery_events_total Total Docker events processed`,
		`# TYPE npm_autodiscovery_events_total counter`,
		`npm_autodiscovery_events_total{action="die",status="success"} 1`,
		`npm_autodiscovery_events_total{action="start",status="success"} 2`,
		`npm_autodiscovery_health_status_total{status="healthy"} 2`,
		`npm_autodiscovery_health_status_total{status="unhealthy"} 1`,
		`# HELP npm_autodiscovery_sync_duration_seconds Latency of reconciliation loop`,
		`# TYPE npm_autodiscovery_sync_duration_seconds histogram`,
		`npm_autodiscovery_sync_duration_seconds_bucket{le="0.05"} 1`,
		`npm_autodiscovery_sync_duration_seconds_bucket{le="0.1"} 2`,
		`npm_autodiscovery_sync_duration_seconds_count 2`,
	}

	for _, line := range expectedLines {
		if !strings.Contains(out, line) {
			t.Errorf("metrics output missing expected line %q:\n%s", line, out)
		}
	}

	// Test HTTP Handler
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("unexpected content type: %s", rr.Header().Get("Content-Type"))
	}
	if !strings.Contains(rr.Body.String(), `npm_autodiscovery_active_proxies{host_id="core-01",source="docker"} 14`) {
		t.Errorf("handler body missing active proxies metric")
	}
}
