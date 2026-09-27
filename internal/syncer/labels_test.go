package syncer

import (
	"strings"
	"testing"

	"github.com/raddadengineer/npm-autodiscovery/internal/config"
	"github.com/raddadengineer/npm-autodiscovery/internal/docker"
)

func TestParseContainerLabels(t *testing.T) {
	cfg := &config.Config{
		LabelPrefix:          "npm.",
		DefaultForwardScheme: "http",
		DefaultSSLEnabled:    false,
		DefaultSSLForced:     false,
		DefaultWebsocket:     true,
		DefaultBlockExploits: true,
	}

	tests := []struct {
		name          string
		inspect       *docker.ContainerInspect
		wantDiscovered bool
		wantDomains   []string
		wantPort      int
		wantScheme    string
		wantSSL       bool
	}{
		{
			name: "Basic valid labels",
			inspect: &docker.ContainerInspect{
				ID:   "abc123456789",
				Name: "/test-web",
				Config: docker.ContainerConfig{
					Labels: map[string]string{
						"npm.frontend.domain": "app.example.com",
						"npm.frontend.port":   "8080",
					},
				},
			},
			wantDiscovered: true,
			wantDomains:   []string{"app.example.com"},
			wantPort:      8080,
			wantScheme:    "http",
			wantSSL:       false,
		},
		{
			name: "Multiple domains with SSL forced and https scheme",
			inspect: &docker.ContainerInspect{
				ID:   "def456789123",
				Name: "/secure-web",
				Config: docker.ContainerConfig{
					Labels: map[string]string{
						"npm.domain":         "secure.example.com, alt.example.com",
						"npm.port":           "8443",
						"npm.forward_scheme": "https",
						"npm.ssl.enabled":    "true",
						"npm.ssl.forced":     "true",
					},
				},
			},
			wantDiscovered: true,
			wantDomains:   []string{"secure.example.com", "alt.example.com"},
			wantPort:      8443,
			wantScheme:    "https",
			wantSSL:       true,
		},
		{
			name: "Missing domain label (should ignore)",
			inspect: &docker.ContainerInspect{
				ID:   "ghi789",
				Name: "/ignored-container",
				Config: docker.ContainerConfig{
					Labels: map[string]string{
						"some.other.label": "foo",
					},
				},
			},
			wantDiscovered: false,
		},
		{
			name: "Explicitly disabled container",
			inspect: &docker.ContainerInspect{
				ID:   "jkl012",
				Name: "/disabled-container",
				Config: docker.ContainerConfig{
					Labels: map[string]string{
						"npm.domain":  "disabled.example.com",
						"npm.port":    "80",
						"npm.enabled": "false",
					},
				},
			},
			wantDiscovered: false,
		},
		{
			name: "Port auto-detected from exposed ports",
			inspect: &docker.ContainerInspect{
				ID:   "mno345",
				Name: "/auto-port-web",
				Config: docker.ContainerConfig{
					Labels: map[string]string{
						"npm.domain": "autoport.example.com",
					},
					ExposedPorts: map[string]interface{}{
						"3000/tcp": struct{}{},
					},
				},
			},
			wantDiscovered: true,
			wantDomains:   []string{"autoport.example.com"},
			wantPort:      3000,
			wantScheme:    "http",
			wantSSL:       false,
		},
		{
			name: "Healthcheck and Middlewares labels",
			inspect: &docker.ContainerInspect{
				ID:   "phase1-container-123",
				Name: "/heavy-webapp",
				Config: docker.ContainerConfig{
					Labels: map[string]string{
						"npm.frontend.domain":             "portal.company.com",
						"npm.frontend.port":               "8080",
						"npm.healthcheck.enabled":         "true",
						"npm.healthcheck.timeout":         "45s",
						"npm.healthcheck.fallback_action": "disable",
						"npm.middleware.strip_prefix":     "/api",
						"npm.middleware.security_headers": "true",
						"npm.middleware.ip_whitelist":     "10.0.0.0/8, 192.168.1.0/24",
					},
				},
			},
			wantDiscovered: true,
			wantDomains:   []string{"portal.company.com"},
			wantPort:      8080,
			wantScheme:    "http",
			wantSSL:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, discovered, err := ParseContainerLabels(tt.inspect, cfg)
			if tt.wantDiscovered && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if discovered != tt.wantDiscovered {
				t.Fatalf("got discovered=%v, want %v", discovered, tt.wantDiscovered)
			}

			if tt.wantDiscovered {
				if len(res.DomainNames) != len(tt.wantDomains) {
					t.Errorf("got domains %v, want %v", res.DomainNames, tt.wantDomains)
				}
				if res.ForwardPort != tt.wantPort {
					t.Errorf("got port %d, want %d", res.ForwardPort, tt.wantPort)
				}
				if res.ForwardScheme != tt.wantScheme {
					t.Errorf("got scheme %s, want %s", res.ForwardScheme, tt.wantScheme)
				}
				if res.SSLEnabled != tt.wantSSL {
					t.Errorf("got ssl %v, want %v", res.SSLEnabled, tt.wantSSL)
				}

				if tt.name == "Healthcheck and Middlewares labels" {
					if !res.HealthCheck.Enabled {
						t.Errorf("expected healthcheck to be enabled")
					}
					if res.HealthCheck.Timeout.Seconds() != 45 {
						t.Errorf("expected 45s healthcheck timeout, got %v", res.HealthCheck.Timeout)
					}
					if res.HealthCheck.FallbackAction != "disable" {
						t.Errorf("expected fallback action disable, got %s", res.HealthCheck.FallbackAction)
					}
					if res.Middlewares == nil {
						t.Fatalf("expected middlewares config to be non-nil")
					}
					if res.Middlewares.StripPrefix != "/api" {
						t.Errorf("expected strip prefix /api, got %s", res.Middlewares.StripPrefix)
					}
					if !res.Middlewares.SecurityHeaders {
						t.Errorf("expected security headers to be true")
					}
					if len(res.Middlewares.IPWhitelist) != 2 {
						t.Errorf("expected 2 ip whitelist entries, got %d", len(res.Middlewares.IPWhitelist))
					}
				}
			}
		})
	}
}

func TestParseCustomLocationsAndSubpaths(t *testing.T) {
	cfg := &config.Config{
		LabelPrefix:          "npm.",
		DefaultForwardScheme: "http",
	}

	// Container with primary subpath /api and custom location /ws
	inspect := &docker.ContainerInspect{
		ID:   "microservice-1",
		Name: "/api-container",
		Config: docker.ContainerConfig{
			Labels: map[string]string{
				"npm.frontend.domain":          "app.company.com",
				"npm.frontend.port":            "8080",
				"npm.frontend.path":            "/api",
				"npm.location.strip_prefix":    "true",
				"npm.location./ws.forward_port": "9000",
				"npm.location./ws.websocket":   "true",
			},
		},
	}

	res, discovered, err := ParseContainerLabels(inspect, cfg)
	if err != nil || !discovered {
		t.Fatalf("expected discovery, got discovered=%v, err=%v", discovered, err)
	}

	if res.Path != "/api" {
		t.Errorf("expected primary path /api, got %s", res.Path)
	}
	if !res.StripPrefix {
		t.Errorf("expected strip_prefix=true")
	}
	if !strings.Contains(res.AdvancedConfig, "rewrite ^/api/?(.*)$ /$1 break;") {
		t.Errorf("expected rewrite in advanced config, got: %s", res.AdvancedConfig)
	}

	if len(res.Locations) != 1 {
		t.Fatalf("expected 1 custom location, got %d", len(res.Locations))
	}

	wsLoc := res.Locations[0]
	if wsLoc.Path != "/ws" || wsLoc.ForwardPort != 9000 {
		t.Errorf("unexpected ws location: %+v", wsLoc)
	}
	if !wsLoc.AllowWebsocketUpgrade {
		t.Errorf("expected websocket upgrade true on ws location")
	}
	if !strings.Contains(wsLoc.AdvancedConfig, "Upgrade $http_upgrade") {
		t.Errorf("expected websocket header in advanced config, got: %s", wsLoc.AdvancedConfig)
	}
}

func TestParseContainerStreams(t *testing.T) {
	cfg := &config.Config{
		LabelPrefix:          "npm.",
		DefaultForwardScheme: "http",
	}

	// Container with primary stream + named stream
	inspect := &docker.ContainerInspect{
		ID:   "minecraft-server-1",
		Name: "/mc-server",
		Config: docker.ContainerConfig{
			Labels: map[string]string{
				"npm.stream.incoming_port":        "25565",
				"npm.stream.forward_port":         "25565",
				"npm.stream.protocol":             "tcp",
				"npm.stream.rcon.incoming_port":    "25575",
				"npm.stream.rcon.forward_port":     "25575",
				"npm.stream.voice.incoming_port":   "9987",
				"npm.stream.voice.protocol":        "udp",
			},
		},
	}

	streams, hasStreams, err := ParseContainerStreams(inspect, cfg)
	if err != nil || !hasStreams {
		t.Fatalf("expected streams, got hasStreams=%v, err=%v", hasStreams, err)
	}

	if len(streams) != 3 {
		t.Fatalf("expected 3 streams, got %d", len(streams))
	}

	portMap := make(map[int]ContainerStreamConfig)
	for _, s := range streams {
		portMap[s.IncomingPort] = s
	}

	mc, ok := portMap[25565]
	if !ok || !mc.TCPForwarding || mc.UDPForwarding {
		t.Errorf("unexpected mc stream: %+v", mc)
	}

	rcon, ok := portMap[25575]
	if !ok || !rcon.TCPForwarding || rcon.UDPForwarding {
		t.Errorf("unexpected rcon stream: %+v", rcon)
	}

	voice, ok := portMap[9987]
	if !ok || voice.TCPForwarding || !voice.UDPForwarding {
		t.Errorf("unexpected voice stream: %+v", voice)
	}
}

func TestParseUpstreamLabels(t *testing.T) {
	cfg := &config.Config{
		LabelPrefix:          "npm.",
		DefaultForwardScheme: "http",
	}

	inspect := &docker.ContainerInspect{
		ID:   "scaled-worker-1",
		Name: "/worker-1",
		Config: docker.ContainerConfig{
			Labels: map[string]string{
				"npm.frontend.domain":   "worker.example.com",
				"npm.frontend.port":     "8080",
				"npm.upstream.algorithm": "ip_hash",
				"npm.upstream.max_fails": "5",
				"npm.upstream.fail_timeout": "30s",
			},
		},
	}

	res, discovered, err := ParseContainerLabels(inspect, cfg)
	if err != nil || !discovered {
		t.Fatalf("expected discovery, got discovered=%v, err=%v", discovered, err)
	}

	if res.Upstream.Algorithm != "ip_hash" {
		t.Errorf("expected ip_hash algorithm, got %s", res.Upstream.Algorithm)
	}
	if res.Upstream.MaxFails != 5 {
		t.Errorf("expected max_fails 5, got %d", res.Upstream.MaxFails)
	}
	if res.Upstream.FailTimeout != "30s" {
		t.Errorf("expected fail_timeout 30s, got %s", res.Upstream.FailTimeout)
	}
}

func TestDuplicatePrefixLabel(t *testing.T) {
	cfg := &config.Config{
		LabelPrefix: "npm.",
	}

	inspect := &docker.ContainerInspect{
		ID:   "dup-prefix-1",
		Name: "/dup-prefix",
		Config: docker.ContainerConfig{
			Labels: map[string]string{
				"npm.frontend.domain":       "vw.halnt.dev",
				"npm.frontend.port":         "8099",
				"npm.npm.certificate_id":    "2",
			},
		},
	}

	res, discovered, err := ParseContainerLabels(inspect, cfg)
	if err != nil || !discovered {
		t.Fatalf("expected discovery, got discovered=%v, err=%v", discovered, err)
	}

	if res.CertificateID != 2 {
		t.Errorf("expected certificate_id 2, got %v", res.CertificateID)
	}
}
