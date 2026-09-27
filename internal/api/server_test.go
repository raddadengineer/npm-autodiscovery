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

func TestClusterReportAndNodesEndpoints(t *testing.T) {
	cfg := &config.Config{
		HostID:       "controller-node",
		HostIP:       "192.168.1.10",
		Port:         8080,
		NPMURL:       "http://127.0.0.1:81",
		ClusterToken: "super-secret-token",
	}

	syncEngine := syncer.NewSyncer(cfg, nil, nil)
	srv := NewServer(cfg, syncEngine, nil)

	// 1. Test POST /api/cluster/report without token (should be 401 Unauthorized)
	reportJSON := `{"node_id":"worker-node-1","node_ip":"192.168.1.50"}`
	req := httptest.NewRequest(http.MethodPost, "/api/cluster/report", strings.NewReader(reportJSON))
	rr := httptest.NewRecorder()
	srv.handleClusterReport(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected HTTP 401 Unauthorized without token, got %d", rr.Code)
	}

	// 2. Test POST /api/cluster/report with valid token
	req = httptest.NewRequest(http.MethodPost, "/api/cluster/report", strings.NewReader(reportJSON))
	req.Header.Set("X-Cluster-Token", "super-secret-token")
	rr = httptest.NewRecorder()
	srv.handleClusterReport(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK with valid token, got %d: %s", rr.Code, rr.Body.String())
	}

	// 3. Test GET /api/cluster/nodes
	req = httptest.NewRequest(http.MethodGet, "/api/cluster/nodes", nil)
	rr = httptest.NewRecorder()
	srv.handleClusterNodes(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK from /api/cluster/nodes, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "controller-node") || !strings.Contains(body, "worker-node-1") {
		t.Errorf("cluster nodes response missing nodes: %s", body)
	}
	if !strings.Contains(body, `"count":2`) {
		t.Errorf("expected count 2 in response: %s", body)
	}

	// 4. Test GET /api/cluster/setup-info
	req = httptest.NewRequest(http.MethodGet, "/api/cluster/setup-info", nil)
	rr = httptest.NewRecorder()
	srv.handleClusterSetupInfo(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK from /api/cluster/setup-info, got %d", rr.Code)
	}

	infoBody := rr.Body.String()
	if !strings.Contains(infoBody, "super-secret-token") {
		t.Errorf("expected cluster_token in setup info: %s", infoBody)
	}
	if !strings.Contains(infoBody, "http://127.0.0.1:81") {
		t.Errorf("expected npm_url in setup info: %s", infoBody)
	}
}


