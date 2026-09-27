package lxd

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseMetadata(t *testing.T) {
	inst := &Instance{
		Name:       "vaultwarden",
		Status:     "Running",
		StatusCode: 103,
		Config: map[string]string{
			"user.npm.domain":                "vault.homelab.local",
			"user.npm.port":                  "8080",
			"user.npm.ssl":                   "true",
			"user.npm.websocket":             "true",
			"user.npm.stream.incoming_port": "3012",
			"user.npm.stream.tcp":            "true",
		},
	}

	route, streams, ok := ExtractRouteAndStreams(
		inst,
		"192.168.1.150",
		"eth0",
		"http",
		false,
		false,
		true,
	)

	if !ok || route == nil {
		t.Fatalf("expected route extraction to succeed")
	}

	if route.Name != "vaultwarden" {
		t.Errorf("expected name vaultwarden, got %s", route.Name)
	}
	if len(route.DomainNames) != 1 || route.DomainNames[0] != "vault.homelab.local" {
		t.Errorf("expected domain vault.homelab.local, got %v", route.DomainNames)
	}
	if route.ForwardPort != 8080 {
		t.Errorf("expected port 8080, got %d", route.ForwardPort)
	}
	if route.ForwardHost != "192.168.1.150" {
		t.Errorf("expected host 192.168.1.150, got %s", route.ForwardHost)
	}
	if !route.SSLEnabled {
		t.Errorf("expected SSLEnabled true")
	}
	if !route.AllowWebsocketUpgrade {
		t.Errorf("expected AllowWebsocketUpgrade true")
	}

	if len(streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(streams))
	}
	s := streams[0]
	if s.IncomingPort != 3012 || s.ForwardingHost != "192.168.1.150" || !s.TCPForwarding {
		t.Errorf("unexpected stream config: %+v", s)
	}
}

func TestResolveInstanceIP(t *testing.T) {
	state := &InstanceState{
		Status:     "Running",
		StatusCode: 103,
		Network: map[string]NetworkInterfaceState{
			"lo": {
				Addresses: []Address{
					{Family: "inet", Address: "127.0.0.1", Scope: "local"},
				},
			},
			"eth0": {
				Addresses: []Address{
					{Family: "inet6", Address: "fe80::1", Scope: "link"},
					{Family: "inet", Address: "192.168.1.200", Scope: "global"},
				},
			},
			"incusbr0": {
				Addresses: []Address{
					{Family: "inet", Address: "10.0.3.50", Scope: "global"},
				},
			},
		},
	}

	subnets, _ := ParseSubnets("192.168.1.0/24")

	// 1. eth0 preferred
	ip, iface, err := ResolveInstanceIP(state, "eth0", subnets)
	if err != nil {
		t.Fatalf("failed resolving IP: %v", err)
	}
	if ip != "192.168.1.200" || iface != "eth0" {
		t.Errorf("expected 192.168.1.200 on eth0, got %s on %s", ip, iface)
	}

	// 2. incusbr0 preferred with matching subnet
	subnets10, _ := ParseSubnets("10.0.0.0/16")
	ip2, iface2, err := ResolveInstanceIP(state, "incusbr0", subnets10)
	if err != nil {
		t.Fatalf("failed resolving IP: %v", err)
	}
	if ip2 != "10.0.3.50" || iface2 != "incusbr0" {
		t.Errorf("expected 10.0.3.50 on incusbr0, got %s on %s", ip2, iface2)
	}

	// 3. No interface matches allowed subnets
	noMatch, _ := ParseSubnets("172.16.0.0/16")
	ip3, _, err := ResolveInstanceIP(state, "eth0", noMatch)
	if err == nil {
		t.Errorf("expected error when no interface matches allowed subnets, got %s", ip3)
	}
}

func TestLXDClientMockSocketServer(t *testing.T) {
	// Create temporary socket
	tmpDir, err := os.MkdirTemp("", "lxd-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sockPath := filepath.Join(tmpDir, "unix.socket")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed creating unix listener: %v", err)
	}
	defer listener.Close()

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			switch r.URL.Path {
			case "/1.0":
				info := ServerInfo{
					APIVersion: "1.0",
					ServerName: "mock-incus",
					Environment: ServerEnvironment{
						Server:        "incus",
						ServerVersion: "6.0.0",
					},
				}
				meta, _ := json.Marshal(info)
				json.NewEncoder(w).Encode(APIResponse{
					Type:       "sync",
					Status:     "Success",
					StatusCode: 200,
					Metadata:   meta,
				})

			case "/1.0/instances":
				insts := []Instance{
					{
						Name:       "nextcloud",
						Status:     "Running",
						StatusCode: 103,
						Config: map[string]string{
							"user.npm.domain": "cloud.homelab.lan",
							"user.npm.port":   "80",
						},
						State: &InstanceState{
							Status:     "Running",
							StatusCode: 103,
							Network: map[string]NetworkInterfaceState{
								"eth0": {
									Addresses: []Address{
										{Family: "inet", Address: "10.0.0.120", Scope: "global"},
									},
								},
							},
						},
					},
				}
				meta, _ := json.Marshal(insts)
				json.NewEncoder(w).Encode(APIResponse{
					Type:       "sync",
					Status:     "Success",
					StatusCode: 200,
					Metadata:   meta,
				})

			default:
				http.NotFound(w, r)
			}
		}),
	}
	go server.Serve(listener)
	defer server.Close()

	client, err := NewClient(sockPath, 2*time.Second, "eth0", "10.0.0.0/16")
	if err != nil {
		t.Fatalf("failed creating LXD client: %v", err)
	}

	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	if client.ServerType() != "incus" {
		t.Errorf("expected serverType incus, got %s", client.ServerType())
	}
	if client.ServerVersion() != "6.0.0" {
		t.Errorf("expected version 6.0.0, got %s", client.ServerVersion())
	}

	routes, streams, instances, err := client.DiscoverRoutes(ctx, "http", false, false, true)
	if err != nil {
		t.Fatalf("DiscoverRoutes failed: %v", err)
	}

	if len(instances) != 1 {
		t.Errorf("expected 1 instance, got %d", len(instances))
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].Name != "nextcloud" || routes[0].ForwardHost != "10.0.0.120" {
		t.Errorf("unexpected route target: %+v", routes[0])
	}
	if len(streams) != 0 {
		t.Errorf("expected 0 streams, got %d", len(streams))
	}
}
