package iac

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRoutesYAML(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "npm-iac-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	yamlContent := `
version: "1.0"
proxy_hosts:
  - domain_names:
      - gitlab.internal.net
      - git.internal.net
    forward_scheme: http
    forward_host: 192.168.1.150
    forward_port: 8080
    ssl_enabled: true
    ssl_forced: true
    allow_websocket_upgrade: true
    block_exploits: true

  - domain_names:
      - nas.local
    forward_host: 192.168.1.200
    forward_port: 5000
`

	manifestPath := filepath.Join(tempDir, "routes.yaml")
	if err := os.WriteFile(manifestPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test yaml: %v", err)
	}

	hosts, err := LoadRoutes(manifestPath)
	if err != nil {
		t.Fatalf("LoadRoutes failed: %v", err)
	}

	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}

	h1 := hosts[0]
	if len(h1.DomainNames) != 2 || h1.DomainNames[0] != "gitlab.internal.net" {
		t.Errorf("unexpected domains for host 1: %v", h1.DomainNames)
	}
	if h1.ForwardHost != "192.168.1.150" || h1.ForwardPort != 8080 {
		t.Errorf("unexpected target for host 1: %s:%d", h1.ForwardHost, h1.ForwardPort)
	}
	if !h1.SSLForced || !h1.AllowWebsocketUpgrade {
		t.Errorf("unexpected flags for host 1: %+v", h1)
	}

	h2 := hosts[1]
	if h2.ForwardHost != "192.168.1.200" || h2.ForwardPort != 5000 {
		t.Errorf("unexpected target for host 2: %s:%d", h2.ForwardHost, h2.ForwardPort)
	}
	if h2.ForwardScheme != "http" {
		t.Errorf("expected default http scheme, got: %s", h2.ForwardScheme)
	}
}

func TestLoadRoutesWithLocations(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "npm-iac-locations-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	yamlContent := `
version: "1.0"
proxy_hosts:
  - domain_names: ["app.example.com"]
    forward_host: "10.0.0.10"
    forward_port: 3000
    locations:
      - path: "/api"
        forward_host: "10.0.0.20"
        forward_port: 8080
        strip_prefix: true
      - path: "/ws"
        forward_host: "10.0.0.30"
        forward_port: 9000
        allow_websocket_upgrade: true
`
	manifestPath := filepath.Join(tempDir, "routes.yaml")
	if err := os.WriteFile(manifestPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test yaml: %v", err)
	}

	hosts, err := LoadRoutes(manifestPath)
	if err != nil {
		t.Fatalf("LoadRoutes failed: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	if len(hosts[0].Locations) != 2 {
		t.Fatalf("expected 2 locations, got %d", len(hosts[0].Locations))
	}

	apiLoc := hosts[0].Locations[0]
	if apiLoc.Path != "/api" || apiLoc.ForwardPort != 8080 {
		t.Errorf("unexpected api location: %+v", apiLoc)
	}
	if apiLoc.AdvancedConfig == "" || !apiLoc.StripPrefix {
		t.Errorf("expected strip_prefix directive in advanced_config, got %s", apiLoc.AdvancedConfig)
	}

	wsLoc := hosts[0].Locations[1]
	if wsLoc.Path != "/ws" || wsLoc.ForwardPort != 9000 {
		t.Errorf("unexpected ws location: %+v", wsLoc)
	}
	if wsLoc.AdvancedConfig == "" || !wsLoc.AllowWebsocketUpgrade {
		t.Errorf("expected websocket upgrade directive in advanced_config, got %s", wsLoc.AdvancedConfig)
	}
}

func TestLoadStreamsYAML(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "npm-iac-streams-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	yamlContent := `
version: "1.0"
streams:
  - incoming_port: 25565
    forwarding_host: 192.168.1.50
    forwarding_port: 25565
    tcp_forwarding: true
    udp_forwarding: false
  - incoming_port: 53
    forwarding_host: 192.168.1.53
    forwarding_port: 53
    tcp_forwarding: true
    udp_forwarding: true
`
	manifestPath := filepath.Join(tempDir, "routes.yaml")
	if err := os.WriteFile(manifestPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test yaml: %v", err)
	}

	streams, err := LoadStreams(manifestPath)
	if err != nil {
		t.Fatalf("LoadStreams failed: %v", err)
	}
	if len(streams) != 2 {
		t.Fatalf("expected 2 streams, got %d", len(streams))
	}

	s1 := streams[0]
	if s1.IncomingPort != 25565 || s1.ForwardingHost != "192.168.1.50" || s1.ForwardingPort != 25565 {
		t.Errorf("unexpected stream 1: %+v", s1)
	}
	if s1.TCPForwarding == nil || !*s1.TCPForwarding || s1.UDPForwarding == nil || *s1.UDPForwarding {
		t.Errorf("unexpected stream 1 protocols")
	}

	s2 := streams[1]
	if s2.IncomingPort != 53 || !*s2.TCPForwarding || !*s2.UDPForwarding {
		t.Errorf("unexpected stream 2: %+v", s2)
	}
}
