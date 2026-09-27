package syncer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/raddadengineer/npm-autodiscovery/internal/config"
	"github.com/raddadengineer/npm-autodiscovery/internal/docker"
	"github.com/raddadengineer/npm-autodiscovery/internal/iac"
	"github.com/raddadengineer/npm-autodiscovery/internal/npm"
)

func TestMicroserviceCustomLocationsAggregation(t *testing.T) {
	var createdPayload *npm.ProxyHostRequest
	var requestCount int32

	mockNPM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tokens":
			_ = json.NewEncoder(w).Encode(npm.TokenResponse{
				Token:   "test-token",
				Expires: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			})
		case "/api/nginx/proxy-hosts":
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode([]npm.ProxyHost{})
			} else if r.Method == http.MethodPost {
				atomic.AddInt32(&requestCount, 1)
				var req npm.ProxyHostRequest
				_ = json.NewDecoder(r.Body).Decode(&req)
				createdPayload = &req
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(npm.ProxyHost{
					ID:          100,
					DomainNames: req.DomainNames,
					ForwardHost: req.ForwardHost,
					ForwardPort: req.ForwardPort,
					Locations:   req.Locations,
				})
			}
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer mockNPM.Close()

	cfg := &config.Config{
		HostID:               "core-01",
		LabelPrefix:          "npm.",
		DefaultForwardScheme: "http",
	}
	npmClient := npm.NewClient(mockNPM.URL, "admin@test.com", "secret", 5*time.Second)
	s := NewSyncer(cfg, nil, npmClient)

	// Container 1: Web UI on /
	c1Inspect := &docker.ContainerInspect{
		ID:   "c1-web",
		Name: "/web-ui",
		Config: docker.ContainerConfig{
			Image: "web-ui:latest",
		},
	}
	c1Cfg := &ContainerProxyConfig{
		ContainerID:   "c1-web",
		ContainerName: "web-ui",
		DomainNames:   []string{"app.company.com"},
		Path:          "/",
		ForwardHost:   "web-ui",
		ForwardPort:   3000,
		ForwardScheme: "http",
	}

	// Container 2: API Service on /api with strip_prefix
	c2Inspect := &docker.ContainerInspect{
		ID:   "c2-api",
		Name: "/api-service",
		Config: docker.ContainerConfig{
			Image: "api-service:latest",
		},
	}
	c2Cfg := &ContainerProxyConfig{
		ContainerID:   "c2-api",
		ContainerName: "api-service",
		DomainNames:   []string{"app.company.com"},
		Path:          "/api",
		StripPrefix:   true,
		ForwardHost:   "api-service",
		ForwardPort:   8080,
		ForwardScheme: "http",
	}

	// Container 3: WebSocket on /ws
	c3Inspect := &docker.ContainerInspect{
		ID:   "c3-ws",
		Name: "/ws-service",
		Config: docker.ContainerConfig{
			Image: "ws-service:latest",
		},
	}
	c3Cfg := &ContainerProxyConfig{
		ContainerID:           "c3-ws",
		ContainerName:         "ws-service",
		DomainNames:           []string{"app.company.com"},
		Path:                  "/ws",
		AllowWebsocketUpgrade: true,
		ForwardHost:           "ws-service",
		ForwardPort:           9000,
		ForwardScheme:         "http",
	}

	eps := []*readyEndpoint{
		{inspect: c1Inspect, proxyCfg: c1Cfg, resMethod: "name"},
		{inspect: c2Inspect, proxyCfg: c2Cfg, resMethod: "name"},
		{inspect: c3Inspect, proxyCfg: c3Cfg, resMethod: "name"},
	}

	s.reconcileDomainGroup(context.Background(), "app.company.com", eps)

	if atomic.LoadInt32(&requestCount) != 1 {
		t.Fatalf("expected 1 create call, got %d", requestCount)
	}

	if createdPayload == nil {
		t.Fatalf("expected createdPayload to be non-nil")
	}

	if createdPayload.ForwardHost != "web-ui" || createdPayload.ForwardPort != 3000 {
		t.Errorf("expected root forward target web-ui:3000, got %s:%d", createdPayload.ForwardHost, createdPayload.ForwardPort)
	}

	if len(createdPayload.Locations) != 2 {
		t.Fatalf("expected 2 locations, got %d", len(createdPayload.Locations))
	}

	locMap := make(map[string]npm.ProxyHostLocation)
	for _, l := range createdPayload.Locations {
		locMap[l.Path] = l
	}

	apiLoc, ok := locMap["/api"]
	if !ok || apiLoc.ForwardHost != "api-service" || apiLoc.ForwardPort != 8080 {
		t.Errorf("unexpected api location: %+v", apiLoc)
	}
	if !strings.Contains(apiLoc.AdvancedConfig, "rewrite ^/api/?(.*)$ /$1 break;") {
		t.Errorf("expected strip_prefix rewrite rule in api location advanced_config, got %s", apiLoc.AdvancedConfig)
	}

	wsLoc, ok := locMap["/ws"]
	if !ok || wsLoc.ForwardHost != "ws-service" || wsLoc.ForwardPort != 9000 {
		t.Errorf("unexpected ws location: %+v", wsLoc)
	}
	if !strings.Contains(wsLoc.AdvancedConfig, "Upgrade $http_upgrade") {
		t.Errorf("expected websocket header in ws location advanced_config, got %s", wsLoc.AdvancedConfig)
	}
}

func TestDynamicUpstreamLoadBalancing(t *testing.T) {
	var createdPayload *npm.ProxyHostRequest

	mockNPM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tokens":
			_ = json.NewEncoder(w).Encode(npm.TokenResponse{
				Token:   "test-token",
				Expires: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			})
		case "/api/nginx/proxy-hosts":
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode([]npm.ProxyHost{})
			} else if r.Method == http.MethodPost {
				var req npm.ProxyHostRequest
				_ = json.NewDecoder(r.Body).Decode(&req)
				createdPayload = &req
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(npm.ProxyHost{
					ID:          200,
					DomainNames: req.DomainNames,
					ForwardHost: req.ForwardHost,
					ForwardPort: req.ForwardPort,
				})
			}
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer mockNPM.Close()

	cfg := &config.Config{
		HostID:               "core-01",
		LabelPrefix:          "npm.",
		DefaultForwardScheme: "http",
	}
	npmClient := npm.NewClient(mockNPM.URL, "admin@test.com", "secret", 5*time.Second)
	s := NewSyncer(cfg, nil, npmClient)

	// 3 Replicas of api-worker on domain api.company.com
	eps := []*readyEndpoint{
		{
			inspect: &docker.ContainerInspect{ID: "rep-1", Name: "/api-worker-1", Config: docker.ContainerConfig{Image: "api:v1"}},
			proxyCfg: &ContainerProxyConfig{
				ContainerID:   "rep-1",
				ContainerName: "api-worker-1",
				DomainNames:   []string{"api.company.com"},
				Path:          "/",
				ForwardHost:   "172.20.0.12",
				ForwardPort:   8080,
				ForwardScheme: "http",
				Upstream: ContainerUpstreamConfig{
					Algorithm:   "least_conn",
					MaxFails:    3,
					FailTimeout: "10s",
				},
			},
			resMethod: "ip",
		},
		{
			inspect: &docker.ContainerInspect{ID: "rep-2", Name: "/api-worker-2", Config: docker.ContainerConfig{Image: "api:v1"}},
			proxyCfg: &ContainerProxyConfig{
				ContainerID:   "rep-2",
				ContainerName: "api-worker-2",
				DomainNames:   []string{"api.company.com"},
				Path:          "/",
				ForwardHost:   "172.20.0.13",
				ForwardPort:   8080,
				ForwardScheme: "http",
				Upstream: ContainerUpstreamConfig{
					Algorithm:   "least_conn",
					MaxFails:    3,
					FailTimeout: "10s",
				},
			},
			resMethod: "ip",
		},
		{
			inspect: &docker.ContainerInspect{ID: "rep-3", Name: "/api-worker-3", Config: docker.ContainerConfig{Image: "api:v1"}},
			proxyCfg: &ContainerProxyConfig{
				ContainerID:   "rep-3",
				ContainerName: "api-worker-3",
				DomainNames:   []string{"api.company.com"},
				Path:          "/",
				ForwardHost:   "172.20.0.14",
				ForwardPort:   8080,
				ForwardScheme: "http",
				Upstream: ContainerUpstreamConfig{
					Algorithm:   "least_conn",
					MaxFails:    3,
					FailTimeout: "10s",
				},
			},
			resMethod: "ip",
		},
	}

	s.reconcileDomainGroup(context.Background(), "api.company.com", eps)

	if createdPayload == nil {
		t.Fatalf("expected createdPayload to be non-nil")
	}

	if createdPayload.ForwardHost != "upstream_api_company_com" {
		t.Errorf("expected ForwardHost to be upstream_api_company_com, got %s", createdPayload.ForwardHost)
	}

	if !strings.Contains(createdPayload.AdvancedConfig, "upstream upstream_api_company_com {") {
		t.Errorf("expected upstream block definition in AdvancedConfig, got:\n%s", createdPayload.AdvancedConfig)
	}
	if !strings.Contains(createdPayload.AdvancedConfig, "server 172.20.0.12:8080 max_fails=3 fail_timeout=10s;") ||
		!strings.Contains(createdPayload.AdvancedConfig, "server 172.20.0.13:8080 max_fails=3 fail_timeout=10s;") ||
		!strings.Contains(createdPayload.AdvancedConfig, "server 172.20.0.14:8080 max_fails=3 fail_timeout=10s;") {
		t.Errorf("expected all 3 replica servers in upstream block, got:\n%s", createdPayload.AdvancedConfig)
	}
}

func TestStreamsReconciliation(t *testing.T) {
	var createdStreams []*npm.StreamRequest
	var deletedStreamIDs []int

	mockNPM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/tokens":
			_ = json.NewEncoder(w).Encode(npm.TokenResponse{
				Token:   "stream-token",
				Expires: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			})
		case r.URL.Path == "/api/nginx/streams" && r.Method == http.MethodGet:
			// Existing orphaned stream on port 9999
			_ = json.NewEncoder(w).Encode([]npm.Stream{
				{
					ID:             50,
					IncomingPort:   9999,
					ForwardingHost: "10.0.0.99",
					ForwardingPort: 9999,
					TCPForwarding:  true,
					Meta: map[string]interface{}{
						"managed_by": ManagedByTag,
						"host_id":    "core-01",
					},
				},
			})
		case r.URL.Path == "/api/nginx/streams" && r.Method == http.MethodPost:
			var req npm.StreamRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			createdStreams = append(createdStreams, &req)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(npm.Stream{
				ID:             int(req.IncomingPort),
				IncomingPort:   req.IncomingPort,
				ForwardingHost: req.ForwardingHost,
				ForwardingPort: req.ForwardingPort,
				TCPForwarding:  npm.FlexBool(req.TCPForwarding),
				UDPForwarding:  npm.FlexBool(req.UDPForwarding),
			})
		case strings.HasPrefix(r.URL.Path, "/api/nginx/streams/50") && r.Method == http.MethodDelete:
			deletedStreamIDs = append(deletedStreamIDs, 50)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer mockNPM.Close()

	cfg := &config.Config{
		HostID:               "core-01",
		LabelPrefix:          "npm.",
		DefaultForwardScheme: "http",
	}
	npmClient := npm.NewClient(mockNPM.URL, "admin@test.com", "secret", 5*time.Second)
	s := NewSyncer(cfg, nil, npmClient)

	dockerStreams := []ContainerStreamConfig{
		{
			Enabled:        true,
			ContainerID:    "c-mc",
			ContainerName:  "minecraft-server",
			IncomingPort:   25565,
			ForwardingHost: "192.168.1.10",
			ForwardingPort: 25565,
			TCPForwarding:  true,
			UDPForwarding:  false,
		},
	}

	tcpTrue := true
	udpTrue := true
	iacStreams := []iac.StaticStream{
		{
			IncomingPort:   53,
			ForwardingHost: "192.168.1.53",
			ForwardingPort: 53,
			TCPForwarding:  &tcpTrue,
			UDPForwarding:  &udpTrue,
			SourceFile:     "dns.yaml",
		},
	}

	synced := s.reconcileStreams(context.Background(), dockerStreams, iacStreams)
	if synced != 2 {
		t.Errorf("expected 2 synced streams, got %d", synced)
	}

	if len(createdStreams) != 2 {
		t.Fatalf("expected 2 created streams, got %d", len(createdStreams))
	}

	if len(deletedStreamIDs) != 1 || deletedStreamIDs[0] != 50 {
		t.Errorf("expected orphan stream 50 to be deleted, got %v", deletedStreamIDs)
	}

	tracked := s.GetTrackedStreams()
	if len(tracked) != 2 {
		t.Errorf("expected 2 tracked streams, got %d", len(tracked))
	}
}
