package middleware

import (
	"fmt"
	"strconv"
	"strings"
)

// Config defines declarative Nginx middleware settings for a proxy route.
type Config struct {
	StripPrefix      string   `json:"strip_prefix,omitempty" yaml:"strip_prefix,omitempty"`
	IPWhitelist      []string `json:"ip_whitelist,omitempty" yaml:"ip_whitelist,omitempty"`
	RateLimit        string   `json:"rate_limit,omitempty" yaml:"rate_limit,omitempty"`
	RateBurst        int      `json:"rate_burst,omitempty" yaml:"rate_burst,omitempty"`
	CORS             bool     `json:"cors,omitempty" yaml:"cors,omitempty"`
	CORSOrigins      []string `json:"cors_origins,omitempty" yaml:"cors_origins,omitempty"`
	SecurityHeaders  bool     `json:"security_headers,omitempty" yaml:"security_headers,omitempty"`
	CustomDirectives []string `json:"custom_directives,omitempty" yaml:"custom_directives,omitempty"`
}

// IsEmpty returns true if no middlewares are active.
func (c *Config) IsEmpty() bool {
	if c == nil {
		return true
	}
	return c.StripPrefix == "" &&
		len(c.IPWhitelist) == 0 &&
		c.RateLimit == "" &&
		!c.CORS &&
		!c.SecurityHeaders &&
		len(c.CustomDirectives) == 0
}

// ParseLabels extracts middleware directives from container labels.
func ParseLabels(labels map[string]string, prefix string) (*Config, bool) {
	if len(labels) == 0 {
		return nil, false
	}

	cfg := &Config{}
	hasAny := false

	// Helper to lookup label by keys
	lookup := func(keys ...string) (string, bool) {
		for _, k := range keys {
			// With configured prefix, e.g. "npm.middleware.strip_prefix"
			fullKey := prefix + "middleware." + k
			if v, ok := labels[fullKey]; ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v), true
			}
			// Without prefix
			directKey := "middleware." + k
			if v, ok := labels[directKey]; ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v), true
			}
		}
		return "", false
	}

	// 1. Strip Prefix
	if val, ok := lookup("strip_prefix", "stripprefix"); ok {
		cfg.StripPrefix = val
		hasAny = true
	}

	// 2. IP Whitelist / CIDR Filtering
	if val, ok := lookup("ip_whitelist", "ip_allowlist", "whitelist", "allowlist"); ok {
		parts := strings.FieldsFunc(val, func(r rune) bool {
			return r == ',' || r == ';' || r == ' '
		})
		for _, p := range parts {
			clean := strings.TrimSpace(p)
			if clean != "" {
				cfg.IPWhitelist = append(cfg.IPWhitelist, clean)
			}
		}
		if len(cfg.IPWhitelist) > 0 {
			hasAny = true
		}
	}

	// 3. Rate Limiting
	if val, ok := lookup("rate_limit", "ratelimit"); ok {
		cfg.RateLimit = val
		hasAny = true
	}
	if val, ok := lookup("rate_burst", "rateburst", "burst"); ok {
		if b, err := strconv.Atoi(val); err == nil && b > 0 {
			cfg.RateBurst = b
		}
	}

	// 4. Automated CORS Headers
	if val, ok := lookup("cors", "enable_cors"); ok {
		if parseBool(val) {
			cfg.CORS = true
			hasAny = true
		}
	}
	if val, ok := lookup("cors_origins", "cors_origin", "allowed_origins"); ok {
		origins := strings.FieldsFunc(val, func(r rune) bool {
			return r == ',' || r == ';'
		})
		for _, o := range origins {
			clean := strings.TrimSpace(o)
			if clean != "" {
				cfg.CORSOrigins = append(cfg.CORSOrigins, clean)
			}
		}
		if len(cfg.CORSOrigins) > 0 {
			cfg.CORS = true
			hasAny = true
		}
	}

	// 5. Security & Privacy Headers
	if val, ok := lookup("security_headers", "securityheaders", "harden"); ok {
		if parseBool(val) {
			cfg.SecurityHeaders = true
			hasAny = true
		}
	}

	if !hasAny {
		return nil, false
	}
	return cfg, true
}

// GenerateNginxConfig synthesizes safe, validated Nginx configuration directives for all active middlewares.
func GenerateNginxConfig(cfg *Config) string {
	if cfg == nil || cfg.IsEmpty() {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# --- Declarative Middlewares (NPM-AutoDiscovery) ---\n")

	// 1. Path Stripping & Regex Rewriting
	if cfg.StripPrefix != "" {
		prefix := strings.TrimSpace(cfg.StripPrefix)
		if !strings.HasPrefix(prefix, "/") {
			prefix = "/" + prefix
		}
		prefix = strings.TrimRight(prefix, "/")
		sb.WriteString(fmt.Sprintf("# Middleware: Strip Prefix (%s)\n", prefix))
		sb.WriteString(fmt.Sprintf("rewrite ^%s/?(.*)$ /$1 break;\n\n", prefix))
	}

	// 2. IP Whitelisting / CIDR Filtering
	if len(cfg.IPWhitelist) > 0 {
		sb.WriteString("# Middleware: IP Whitelist / CIDR Filtering\n")
		for _, ip := range cfg.IPWhitelist {
			sb.WriteString(fmt.Sprintf("allow %s;\n", ip))
		}
		sb.WriteString("deny all;\n\n")
	}

	// 3. Token-Bucket Rate Limiting
	if cfg.RateLimit != "" {
		burst := cfg.RateBurst
		if burst <= 0 {
			burst = 10 // sensible default burst
		}
		sb.WriteString(fmt.Sprintf("# Middleware: Token-Bucket Rate Limiting (%s)\n", cfg.RateLimit))
		sb.WriteString(fmt.Sprintf("limit_req zone=npm_zone_$server_name burst=%d nodelay;\n\n", burst))
	}

	// 4. Automated CORS Headers
	if cfg.CORS {
		sb.WriteString("# Middleware: Automated CORS Headers\n")
		sb.WriteString("if ($request_method = 'OPTIONS') {\n")
		sb.WriteString("    add_header 'Access-Control-Allow-Origin' '$http_origin' always;\n")
		sb.WriteString("    add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS, PUT, DELETE, PATCH' always;\n")
		sb.WriteString("    add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,Authorization' always;\n")
		sb.WriteString("    add_header 'Access-Control-Max-Age' 1728000;\n")
		sb.WriteString("    add_header 'Content-Type' 'text/plain; charset=utf-8';\n")
		sb.WriteString("    add_header 'Content-Length' 0;\n")
		sb.WriteString("    return 204;\n")
		sb.WriteString("}\n")
		sb.WriteString("add_header 'Access-Control-Allow-Origin' '$http_origin' always;\n")
		sb.WriteString("add_header 'Access-Control-Allow-Credentials' 'true' always;\n\n")
	}

	// 5. Security & Privacy Headers
	if cfg.SecurityHeaders {
		sb.WriteString("# Middleware: Security & Privacy Headers\n")
		sb.WriteString("add_header X-Frame-Options \"SAMEORIGIN\" always;\n")
		sb.WriteString("add_header X-XSS-Protection \"1; mode=block\" always;\n")
		sb.WriteString("add_header X-Content-Type-Options \"nosniff\" always;\n")
		sb.WriteString("add_header Referrer-Policy \"strict-origin-when-cross-origin\" always;\n\n")
	}

	// 6. Custom Directives
	for _, cd := range cfg.CustomDirectives {
		clean := strings.TrimSpace(cd)
		if clean != "" {
			if !strings.HasSuffix(clean, ";") && !strings.HasSuffix(clean, "}") {
				clean += ";"
			}
			sb.WriteString(clean + "\n")
		}
	}

	return strings.TrimSpace(sb.String())
}

func parseBool(str string) bool {
	str = strings.ToLower(strings.TrimSpace(str))
	return str == "true" || str == "1" || str == "yes" || str == "on"
}
