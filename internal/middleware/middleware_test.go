package middleware

import (
	"strings"
	"testing"
)

func TestParseLabels(t *testing.T) {
	prefix := "npm."

	tests := []struct {
		name         string
		labels       map[string]string
		wantConfig   bool
		checkConfig  func(*testing.T, *Config)
	}{
		{
			name:       "Empty labels",
			labels:     map[string]string{},
			wantConfig: false,
		},
		{
			name: "Strip prefix label",
			labels: map[string]string{
				"npm.middleware.strip_prefix": "/api/v1",
			},
			wantConfig: true,
			checkConfig: func(t *testing.T, c *Config) {
				if c.StripPrefix != "/api/v1" {
					t.Errorf("got strip_prefix %q, want /api/v1", c.StripPrefix)
				}
			},
		},
		{
			name: "IP Whitelist comma-separated",
			labels: map[string]string{
				"npm.middleware.ip_whitelist": "192.168.1.0/24, 10.0.0.0/8, 172.16.0.5",
			},
			wantConfig: true,
			checkConfig: func(t *testing.T, c *Config) {
				if len(c.IPWhitelist) != 3 {
					t.Fatalf("got %d IP whitelist entries, want 3", len(c.IPWhitelist))
				}
				if c.IPWhitelist[0] != "192.168.1.0/24" || c.IPWhitelist[1] != "10.0.0.0/8" || c.IPWhitelist[2] != "172.16.0.5" {
					t.Errorf("unexpected IP whitelist: %v", c.IPWhitelist)
				}
			},
		},
		{
			name: "Rate limiting with burst",
			labels: map[string]string{
				"npm.middleware.rate_limit": "10r/s",
				"npm.middleware.rate_burst": "20",
			},
			wantConfig: true,
			checkConfig: func(t *testing.T, c *Config) {
				if c.RateLimit != "10r/s" || c.RateBurst != 20 {
					t.Errorf("got rate_limit %s burst %d, want 10r/s burst 20", c.RateLimit, c.RateBurst)
				}
			},
		},
		{
			name: "CORS and Security Headers enabled",
			labels: map[string]string{
				"npm.middleware.cors":             "true",
				"npm.middleware.cors_origins":     "https://app.example.com, https://admin.example.com",
				"npm.middleware.security_headers": "true",
			},
			wantConfig: true,
			checkConfig: func(t *testing.T, c *Config) {
				if !c.CORS {
					t.Errorf("CORS should be enabled")
				}
				if len(c.CORSOrigins) != 2 {
					t.Errorf("got %d CORS origins, want 2", len(c.CORSOrigins))
				}
				if !c.SecurityHeaders {
					t.Errorf("SecurityHeaders should be enabled")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, ok := ParseLabels(tt.labels, prefix)
			if ok != tt.wantConfig {
				t.Fatalf("got ok=%v, want %v", ok, tt.wantConfig)
			}
			if ok && tt.checkConfig != nil {
				tt.checkConfig(t, cfg)
			}
		})
	}
}

func TestGenerateNginxConfig(t *testing.T) {
	cfg := &Config{
		StripPrefix:     "/api",
		IPWhitelist:     []string{"192.168.1.0/24", "10.0.0.0/8", "172.16.0.5"},
		RateLimit:       "10r/s",
		RateBurst:       20,
		CORS:            true,
		SecurityHeaders: true,
	}

	out := GenerateNginxConfig(cfg)

	// Check for all synthesized directives matching ROADMAP.md Section 1.2
	expectedSnippets := []string{
		"rewrite ^/api/?(.*)$ /$1 break;",
		"allow 192.168.1.0/24;",
		"allow 10.0.0.0/8;",
		"allow 172.16.0.5;",
		"deny all;",
		"limit_req zone=npm_zone_$server_name burst=20 nodelay;",
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"X-Frame-Options \"SAMEORIGIN\" always;",
		"X-XSS-Protection \"1; mode=block\" always;",
		"X-Content-Type-Options \"nosniff\" always;",
		"Referrer-Policy \"strict-origin-when-cross-origin\" always;",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(out, snippet) {
			t.Errorf("generated nginx config missing expected snippet %q:\n%s", snippet, out)
		}
	}
}

func TestGenerateNginxConfigEmpty(t *testing.T) {
	if out := GenerateNginxConfig(nil); out != "" {
		t.Errorf("expected empty string for nil config, got %q", out)
	}
	if out := GenerateNginxConfig(&Config{}); out != "" {
		t.Errorf("expected empty string for empty config, got %q", out)
	}
}
