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
