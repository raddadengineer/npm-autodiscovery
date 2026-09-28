package api

import (
	"encoding/json"
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

func TestProxmoxEndpoints(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	cfg := &config.Config{
		HostID:       "controller-node",
		HostIP:       "192.168.1.10",
		Port:         8080,
		NPMURL:       "http://127.0.0.1:81",
		ClusterToken: "token-123",
	}

	syncEngine := syncer.NewSyncer(cfg, nil, nil)
	srv := NewServer(cfg, syncEngine, nil)

	// 1. Test POST /api/cluster/nodes/worker-node-1/proxmox
	pveJSON := `{
		"enabled": true,
		"url": "https://192.168.1.100:8006",
		"token_id": "root@pam!npm",
		"token_secret": "my-secret-uuid-1234",
		"node": "pve",
		"verify_ssl": false,
		"preferred_interface": "eth0"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/cluster/nodes/worker-node-1/proxmox", strings.NewReader(pveJSON))
	req.SetPathValue("nodeId", "worker-node-1")
	rr := httptest.NewRecorder()
	srv.handleNodeProxmox(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// 2. Test GET /api/cluster/nodes/worker-node-1/proxmox
	req = httptest.NewRequest(http.MethodGet, "/api/cluster/nodes/worker-node-1/proxmox", nil)
	req.SetPathValue("nodeId", "worker-node-1")
	rr = httptest.NewRecorder()
	srv.handleNodeProxmox(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"enabled":true`) || !strings.Contains(body, "https://192.168.1.100:8006") {
		t.Errorf("unexpected GET proxmox config response: %s", body)
	}
	// Verify token_secret is masked in GET response
	if strings.Contains(body, "my-secret-uuid-1234") {
		t.Errorf("token_secret should be masked in GET response, but was leaked: %s", body)
	}

	// 3. Test POST /api/cluster/report returns pve_config to worker
	reportJSON := `{"node_id":"worker-node-1","node_ip":"192.168.1.50"}`
	req = httptest.NewRequest(http.MethodPost, "/api/cluster/report", strings.NewReader(reportJSON))
	req.Header.Set("X-Cluster-Token", "token-123")
	rr = httptest.NewRecorder()
	srv.handleClusterReport(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rr.Code, rr.Body.String())
	}
	reportBody := rr.Body.String()
	if !strings.Contains(reportBody, `"pve_config"`) || !strings.Contains(reportBody, "https://192.168.1.100:8006") {
		t.Errorf("expected pve_config in cluster report response: %s", reportBody)
	}

	// 4. Test handleProxmoxTest with empty URL
	req = httptest.NewRequest(http.MethodPost, "/api/proxmox/test", strings.NewReader(`{"url":""}`))
	rr = httptest.NewRecorder()
	srv.handleProxmoxTest(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected HTTP 400 for empty URL, got %d", rr.Code)
	}

	// 5. Test adding a second PVE endpoint to worker-node-1
	pveJSON2 := `{
		"id": "pve-backup",
		"name": "Backup Proxmox",
		"enabled": true,
		"url": "https://192.168.1.200:8006",
		"token_id": "root@pam!backup",
		"token_secret": "my-secret-2",
		"node": "pve2"
	}`
	req = httptest.NewRequest(http.MethodPost, "/api/cluster/nodes/worker-node-1/proxmox", strings.NewReader(pveJSON2))
	req.SetPathValue("nodeId", "worker-node-1")
	rr = httptest.NewRecorder()
	srv.handleNodeProxmox(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 adding 2nd PVE config, got %d: %s", rr.Code, rr.Body.String())
	}

	// 6. Test GET returns both configs
	req = httptest.NewRequest(http.MethodGet, "/api/cluster/nodes/worker-node-1/proxmox", nil)
	req.SetPathValue("nodeId", "worker-node-1")
	rr = httptest.NewRecorder()
	srv.handleNodeProxmox(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rr.Code)
	}
	var getResp struct {
		Configs []syncer.PVEConfig `json:"configs"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed decoding GET response: %v", err)
	}
	if len(getResp.Configs) != 2 {
		t.Fatalf("expected 2 configs in GET response, got %d", len(getResp.Configs))
	}

	// 7. Test DELETE endpoint
	req = httptest.NewRequest(http.MethodDelete, "/api/cluster/nodes/worker-node-1/proxmox?id=pve-backup", nil)
	req.SetPathValue("nodeId", "worker-node-1")
	rr = httptest.NewRecorder()
	srv.handleNodeProxmox(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 on DELETE, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify count is now 1
	cfgsNow := syncEngine.GetNodePVEConfigs("worker-node-1")
	if len(cfgsNow) != 1 {
		t.Fatalf("expected 1 config after DELETE, got %d", len(cfgsNow))
	}
}



