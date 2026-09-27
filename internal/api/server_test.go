package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/raddadengineer/npm-autodiscovery/internal/config"
	"github.com/raddadengineer/npm-autodiscovery/internal/metrics"
	"github.com/raddadengineer/npm-autodiscovery/internal/syncer"
)

func TestMetricsEndpoint(t *testing.T) {
	cfg := &config.Config{
		HostID:  "node-metrics-test",
		Port:    8080,
		NPMURL:  "http://127.0.0.1:81",
		NPMUser: "test@example.com",
	}

	metrics.SetActiveProxies("node-metrics-test", "docker", 5)
	metrics.IncEvents("start", "success")

	srv := &Server{
		cfg:    cfg,
		syncer: &syncer.Syncer{},
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	srv.handleMetrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "npm_autodiscovery_active_proxies") {
		t.Errorf("metrics response missing npm_autodiscovery_active_proxies:\n%s", body)
	}
	if !strings.Contains(body, "npm_autodiscovery_events_total") {
		t.Errorf("metrics response missing npm_autodiscovery_events_total:\n%s", body)
	}
}

func TestConfigEndpoint(t *testing.T) {
	cfg := &config.Config{
		HostID:                     "node-test",
		NPMURL:                     "http://127.0.0.1:81",
		DefaultHealthcheckEnabled:  true,
		DefaultHealthcheckFallback: "disable",
	}

	srv := &Server{
		cfg: cfg,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rr := httptest.NewRecorder()

	srv.handleConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "node-test") {
		t.Errorf("config response missing host_id: %s", body)
	}
	if !strings.Contains(body, "default_healthcheck_enabled") {
		t.Errorf("config response missing default_healthcheck_enabled: %s", body)
	}
}

func TestStreamsEndpoint(t *testing.T) {
	srv := &Server{
		syncer: &syncer.Syncer{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/streams", nil)
	rr := httptest.NewRecorder()

	srv.handleStreams(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, `"streams":[]`) && !strings.Contains(body, `"count":0`) {
		t.Errorf("unexpected streams response: %s", body)
	}
}

