package pve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParsePVETags(t *testing.T) {
	tags := "npm-domain=adguard.homelab.arpa;npm-port=80;npm-ssl=true;npm-websocket=true"
	parsed := ParsePVETags(tags)

	if parsed["npm.domain"] != "adguard.homelab.arpa" {
		t.Errorf("expected domain adguard.homelab.arpa, got %s", parsed["npm.domain"])
	}
	if parsed["npm.port"] != "80" {
		t.Errorf("expected port 80, got %s", parsed["npm.port"])
	}
	if parsed["npm.ssl"] != "true" {
		t.Errorf("expected ssl true, got %s", parsed["npm.ssl"])
	}
	if parsed["npm.websocket"] != "true" {
		t.Errorf("expected websocket true, got %s", parsed["npm.websocket"])
	}
}

func TestParsePVENotesYAML(t *testing.T) {
	notes := `
# NPM Discovery Configuration
npm.domain: vault.homelab.arpa
npm.port: 8000
npm.ssl: true
npm.websocket: true
npm.stream.incoming_port: 25565
`
	parsed := ParsePVENotes(notes)
	if parsed["npm.domain"] != "vault.homelab.arpa" {
		t.Errorf("expected domain vault.homelab.arpa, got %s", parsed["npm.domain"])
	}
	if parsed["npm.port"] != "8000" {
		t.Errorf("expected port 8000, got %s", parsed["npm.port"])
	}
	if parsed["npm.ssl"] != "true" {
		t.Errorf("expected ssl true, got %s", parsed["npm.ssl"])
	}
	if parsed["npm.stream.incoming_port"] != "25565" {
		t.Errorf("expected stream port 25565, got %s", parsed["npm.stream.incoming_port"])
	}
}

func TestExtractRouteAndStreams(t *testing.T) {
	tags := "npm-domain=adguard.lan;npm-port=80"
	notes := "npm.ssl: true\nnpm.websocket: true\n"

	route, streams, ok := ExtractRouteAndStreams(
		101,
		"pve-node-01",
		"adguard",
		"192.168.1.101",
		"eth0",
		tags,
		notes,
		"http",
		false,
		false,
		true,
	)

	if !ok || route == nil {
		t.Fatalf("expected route extraction to succeed")
	}

	if route.VMID != 101 {
		t.Errorf("expected VMID 101, got %d", route.VMID)
	}
	if len(route.DomainNames) != 1 || route.DomainNames[0] != "adguard.lan" {
		t.Errorf("expected domain adguard.lan, got %v", route.DomainNames)
	}
	if route.ForwardPort != 80 {
		t.Errorf("expected port 80, got %d", route.ForwardPort)
	}
	if route.ForwardHost != "192.168.1.101" {
		t.Errorf("expected host 192.168.1.101, got %s", route.ForwardHost)
	}
	if !route.SSLEnabled {
		t.Errorf("expected SSLEnabled true (from notes)")
	}
	if !route.AllowWebsocketUpgrade {
		t.Errorf("expected Websocket true (from notes)")
	}
	if route.DiscoveryChannel != "both" {
		t.Errorf("expected discovery channel 'both', got %s", route.DiscoveryChannel)
	}
	if len(streams) != 0 {
		t.Errorf("expected 0 streams, got %d", len(streams))
	}
}

func TestIPResolutionAndSubnetMatching(t *testing.T) {
	subnets, err := ParseSubnets("192.168.1.0/24,10.0.0.0/16")
	if err != nil {
		t.Fatalf("failed parsing subnets: %v", err)
	}

	cleanIP, ipObj, err := ExtractCleanIPv4("192.168.1.50/24")
	if err != nil {
		t.Fatalf("failed extracting IP: %v", err)
	}
	if cleanIP != "192.168.1.50" {
		t.Errorf("expected 192.168.1.50, got %s", cleanIP)
	}

	if !MatchSubnets(ipObj, subnets) {
		t.Errorf("expected 192.168.1.50 to match subnets")
	}

	_, ipObj2, _ := ExtractCleanIPv4("172.16.0.5")
	if MatchSubnets(ipObj2, subnets) {
		t.Errorf("expected 172.16.0.5 NOT to match subnets")
	}
}

func TestPVEClientMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "PVEAPIToken=test@pve!token1=secret123" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api2/json/version":
			resp := APIResponse{
				Data: json.RawMessage(`{"version": "8.1-1", "release": "8.1", "repoid": "abc"}`),
			}
			json.NewEncoder(w).Encode(resp)

		case "/api2/json/nodes":
			resp := APIResponse{
				Data: json.RawMessage(`[{"node": "pve-01", "status": "online"}]`),
			}
			json.NewEncoder(w).Encode(resp)

		case "/api2/json/nodes/pve-01/lxc":
			resp := APIResponse{
				Data: json.RawMessage(`[
					{"vmid": 101, "name": "adguard", "status": "running", "tags": "npm-domain=dns.lan;npm-port=80;npm-ssl=true"},
					{"vmid": 102, "name": "db", "status": "stopped", "tags": "npm-domain=db.lan"}
				]`),
			}
			json.NewEncoder(w).Encode(resp)

		case "/api2/json/nodes/pve-01/lxc/101/config":
			resp := APIResponse{
				Data: json.RawMessage(`{
					"description": "npm.websocket: true\nnpm.stream.incoming_port: 53\nnpm.stream.udp: true\n",
					"net0": "name=eth0,bridge=vmbr0,ip=192.168.1.101/24"
				}`),
			}
			json.NewEncoder(w).Encode(resp)

		case "/api2/json/nodes/pve-01/lxc/101/interfaces":
			resp := APIResponse{
				Data: json.RawMessage(`[
					{"name": "lo", "inet": "127.0.0.1/8"},
					{"name": "eth0", "inet": "192.168.1.101/24"}
				]`),
			}
			json.NewEncoder(w).Encode(resp)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(
		server.URL,
		"test@pve!token1",
		"secret123",
		"pve-01",
		false,
		5*time.Second,
		"eth0",
		"192.168.1.0/24",
	)
	if err != nil {
		t.Fatalf("failed initializing PVE client: %v", err)
	}

	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if !client.IsConnected() {
		t.Errorf("expected connected true")
	}

	routes, streams, containers, err := client.DiscoverRoutes(ctx, "http", false, false, true)
	if err != nil {
		t.Fatalf("DiscoverRoutes failed: %v", err)
	}

	if len(containers) != 2 {
		t.Errorf("expected 2 containers, got %d", len(containers))
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	r := routes[0]
	if r.VMID != 101 || r.ContainerName != "adguard" {
		t.Errorf("unexpected route target: %+v", r)
	}
	if r.ForwardHost != "192.168.1.101" || r.ForwardPort != 80 {
		t.Errorf("unexpected host/port: %s:%d", r.ForwardHost, r.ForwardPort)
	}
	if !r.SSLEnabled {
		t.Errorf("expected SSLEnabled true")
	}
	if !r.AllowWebsocketUpgrade {
		t.Errorf("expected AllowWebsocketUpgrade true")
	}

	if len(streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(streams))
	}
	s := streams[0]
	if s.IncomingPort != 53 || s.ForwardingHost != "192.168.1.101" || !s.UDPForwarding {
		t.Errorf("unexpected stream config: %+v", s)
	}
}
