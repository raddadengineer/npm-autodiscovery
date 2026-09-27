package syncer

import (
	"testing"

	"github.com/sampson/npm-autodiscovery/internal/config"
	"github.com/sampson/npm-autodiscovery/internal/docker"
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
			}
		})
	}
}
