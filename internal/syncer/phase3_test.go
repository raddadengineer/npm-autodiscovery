package syncer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/raddadengineer/npm-autodiscovery/internal/config"
	"github.com/raddadengineer/npm-autodiscovery/internal/lxd"
	"github.com/raddadengineer/npm-autodiscovery/internal/npm"
	"github.com/raddadengineer/npm-autodiscovery/internal/pve"
)

func TestPVEReconciliation(t *testing.T) {
	mockNPM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tokens":
			_ = json.NewEncoder(w).Encode(npm.TokenResponse{
				Token:   "test-token",
				Expires: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			})
			return
		case "/api/nginx/proxy-hosts":
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode([]npm.ProxyHost{})
				return
			}
			if r.Method == http.MethodPost {
				var req npm.ProxyHostRequest
				json.NewDecoder(r.Body).Decode(&req)
				json.NewEncoder(w).Encode(npm.ProxyHost{
					ID:            301,
					DomainNames:   req.DomainNames,
					ForwardScheme: req.ForwardScheme,
					ForwardHost:   req.ForwardHost,
					ForwardPort:   req.ForwardPort,
					Meta:          req.Meta,
				})
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer mockNPM.Close()

	cfg := &config.Config{
		NPMURL:     mockNPM.URL,
		HostID:     "cluster-node-01",
		PVEEnabled: true,
	}
	npmClient := npm.NewClient(cfg.NPMURL, "test", "test", 2*time.Second)
	s := NewSyncer(cfg, nil, npmClient)

	route := pve.PVERoute{
		VMID:          101,
		Node:          "pve-01",
		ContainerName: "adguard-home",
		DomainNames:   []string{"adguard.homelab.arpa"},
		ForwardScheme: "http",
		ForwardHost:   "192.168.1.101",
		ForwardPort:   80,
		Interface:     "eth0",
		SSLEnabled:    true,
	}

	ctx := context.Background()
	s.reconcilePVERoute(ctx, route)

	proxies := s.GetTrackedProxies()
	if len(proxies) != 1 {
		t.Fatalf("expected 1 tracked proxy, got %d", len(proxies))
	}
	p := proxies[0]
	if p.Source != "pve" {
		t.Errorf("expected source 'pve', got %s", p.Source)
	}
	if p.ForwardHost != "192.168.1.101" || p.ForwardPort != 80 {
		t.Errorf("expected 192.168.1.101:80, got %s:%d", p.ForwardHost, p.ForwardPort)
	}
	if p.ContainerID != "pve:101" {
		t.Errorf("expected containerID 'pve:101', got %s", p.ContainerID)
	}
}

func TestLXDReconciliation(t *testing.T) {
	mockNPM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tokens":
			_ = json.NewEncoder(w).Encode(npm.TokenResponse{
				Token:   "test-token",
				Expires: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			})
			return
		case "/api/nginx/proxy-hosts":
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode([]npm.ProxyHost{})
				return
			}
			if r.Method == http.MethodPost {
				var req npm.ProxyHostRequest
				json.NewDecoder(r.Body).Decode(&req)
				json.NewEncoder(w).Encode(npm.ProxyHost{
					ID:            401,
					DomainNames:   req.DomainNames,
					ForwardScheme: req.ForwardScheme,
					ForwardHost:   req.ForwardHost,
					ForwardPort:   req.ForwardPort,
					Meta:          req.Meta,
				})
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer mockNPM.Close()

	cfg := &config.Config{
		NPMURL:     mockNPM.URL,
		HostID:     "cluster-node-01",
		LXDEnabled: true,
	}
	npmClient := npm.NewClient(cfg.NPMURL, "test", "test", 2*time.Second)
	s := NewSyncer(cfg, nil, npmClient)

	route := lxd.LXDRoute{
		Name:          "vaultwarden",
		DomainNames:   []string{"vault.homelab.arpa"},
		ForwardScheme: "http",
		ForwardHost:   "10.0.3.50",
		ForwardPort:   8080,
		Interface:     "incusbr0",
		SSLEnabled:    true,
	}

	ctx := context.Background()
	s.reconcileLXDRoute(ctx, route)

	proxies := s.GetTrackedProxies()
	if len(proxies) != 1 {
		t.Fatalf("expected 1 tracked proxy, got %d", len(proxies))
	}
	p := proxies[0]
	if p.Source != "lxd" {
		t.Errorf("expected source 'lxd', got %s", p.Source)
	}
	if p.ForwardHost != "10.0.3.50" || p.ForwardPort != 8080 {
		t.Errorf("expected 10.0.3.50:8080, got %s:%d", p.ForwardHost, p.ForwardPort)
	}
	if p.ContainerID != "lxd:vaultwarden" {
		t.Errorf("expected containerID 'lxd:vaultwarden', got %s", p.ContainerID)
	}
}

func TestMultiProviderStreamsReconciliation(t *testing.T) {
	createdStreams := make(map[int]npm.StreamRequest)

	mockNPM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tokens":
			_ = json.NewEncoder(w).Encode(npm.TokenResponse{
				Token:   "test-token",
				Expires: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			})
			return
		case "/api/nginx/streams":
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode([]npm.Stream{})
				return
			}
			if r.Method == http.MethodPost {
				var req npm.StreamRequest
				json.NewDecoder(r.Body).Decode(&req)
				createdStreams[req.IncomingPort] = req
				json.NewEncoder(w).Encode(npm.Stream{
					ID:             req.IncomingPort,
					IncomingPort:   req.IncomingPort,
					ForwardingHost: req.ForwardingHost,
					ForwardingPort: req.ForwardingPort,
					TCPForwarding:  npm.FlexBool(req.TCPForwarding),
					UDPForwarding:  npm.FlexBool(req.UDPForwarding),
					Meta:           req.Meta,
				})
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer mockNPM.Close()

	cfg := &config.Config{
		NPMURL: mockNPM.URL,
		HostID: "core-node",
	}
	npmClient := npm.NewClient(cfg.NPMURL, "test", "test", 2*time.Second)
	s := NewSyncer(cfg, nil, npmClient)

	dockerStreams := []ContainerStreamConfig{
		{
			IncomingPort:   25565,
			ForwardingHost: "192.168.1.10",
			ForwardingPort: 25565,
			TCPForwarding:  true,
		},
	}
	pveStreams := []pve.PVEStream{
		{
			VMID:           101,
			Node:           "pve-01",
			ContainerName:  "pihole",
			IncomingPort:   53,
			ForwardingHost: "192.168.1.53",
			ForwardingPort: 53,
			TCPForwarding:  true,
			UDPForwarding:  true,
		},
	}
	lxdStreams := []lxd.LXDStream{
		{
			Name:           "mqtt-broker",
			IncomingPort:   1883,
			ForwardingHost: "10.0.3.183",
			ForwardingPort: 1883,
			TCPForwarding:  true,
		},
	}

	ctx := context.Background()
	synced := s.reconcileStreams(ctx, dockerStreams, nil, pveStreams, lxdStreams)

	if synced != 3 {
		t.Errorf("expected 3 streams synced, got %d", synced)
	}

	tracked := s.GetTrackedStreams()
	if len(tracked) != 3 {
		t.Fatalf("expected 3 tracked streams, got %d", len(tracked))
	}

	sources := make(map[string]bool)
	for _, st := range tracked {
		sources[st.Source] = true
	}

	if !sources["docker"] || !sources["pve"] || !sources["lxd"] {
		t.Errorf("expected streams from docker, pve, and lxd, got: %+v", tracked)
	}
}

func TestStatusOverviewHypervisors(t *testing.T) {
	cfg := &config.Config{
		NPMURL:     "http://mock-npm:81",
		HostID:     "cluster-node",
		PVEEnabled: true,
		LXDEnabled: true,
	}
	s := NewSyncer(cfg, nil, nil)

	s.mu.Lock()
	s.pveContainers = []pve.LXCContainerSummary{
		{VMID: 101, Name: "dns", Status: "running"},
		{VMID: 102, Name: "db", Status: "stopped"},
	}
	s.lxdInstances = []lxd.Instance{
		{Name: "web", Status: "Running", StatusCode: 103},
	}
	s.mu.Unlock()

	overview := s.GetStatusOverview(context.Background())
	if !overview.PVEEnabled || !overview.LXDEnabled {
		t.Errorf("expected PVE and LXD enabled in status overview")
	}

	views, err := s.GetContainersView(context.Background())
	if err != nil {
		t.Fatalf("failed GetContainersView: %v", err)
	}

	if len(views) != 3 {
		t.Errorf("expected 3 container views (2 PVE + 1 LXD), got %d", len(views))
	}

	foundPVE := false
	foundLXD := false
	for _, v := range views {
		if v.Source == "pve" {
			foundPVE = true
		}
		if v.Source == "lxd" {
			foundLXD = true
		}
	}

	if !foundPVE || !foundLXD {
		t.Errorf("expected container views to include both pve and lxd sources")
	}
}
