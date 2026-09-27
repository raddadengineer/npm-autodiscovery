package iac

import "github.com/sampson/npm-autodiscovery/internal/middleware"

// StaticLocation defines path sub-routing within a static proxy host.
type StaticLocation struct {
	Path                  string             `json:"path" yaml:"path"`
	ForwardScheme         string             `json:"forward_scheme,omitempty" yaml:"forward_scheme,omitempty"`
	ForwardHost           string             `json:"forward_host" yaml:"forward_host"`
	ForwardPort           int                `json:"forward_port" yaml:"forward_port"`
	AdvancedConfig        string             `json:"advanced_config,omitempty" yaml:"advanced_config,omitempty"`
	StripPrefix           bool               `json:"strip_prefix,omitempty" yaml:"strip_prefix,omitempty"`
	AllowWebsocketUpgrade bool               `json:"allow_websocket_upgrade,omitempty" yaml:"allow_websocket_upgrade,omitempty"`
	Middlewares           *middleware.Config `json:"middlewares,omitempty" yaml:"middlewares,omitempty"`
}

// StaticProxyHost represents a declarative proxy host definition loaded from a YAML or JSON file.
type StaticProxyHost struct {
	DomainNames           []string           `json:"domain_names" yaml:"domain_names"`
	ForwardScheme         string             `json:"forward_scheme,omitempty" yaml:"forward_scheme,omitempty"`
	ForwardHost           string             `json:"forward_host" yaml:"forward_host"`
	ForwardPort           int                `json:"forward_port" yaml:"forward_port"`
	CertificateID         interface{}        `json:"certificate_id,omitempty" yaml:"certificate_id,omitempty"`
	SSLEnabled            bool               `json:"ssl_enabled,omitempty" yaml:"ssl_enabled,omitempty"`
	SSLForced             bool               `json:"ssl_forced,omitempty" yaml:"ssl_forced,omitempty"`
	AllowWebsocketUpgrade bool               `json:"allow_websocket_upgrade,omitempty" yaml:"allow_websocket_upgrade,omitempty"`
	BlockExploits         bool               `json:"block_exploits,omitempty" yaml:"block_exploits,omitempty"`
	CachingEnabled        bool               `json:"caching_enabled,omitempty" yaml:"caching_enabled,omitempty"`
	HTTP2Support          bool               `json:"http2_support,omitempty" yaml:"http2_support,omitempty"`
	HSTSEnabled           bool               `json:"hsts_enabled,omitempty" yaml:"hsts_enabled,omitempty"`
	HSTSSubdomains        bool               `json:"hsts_subdomains,omitempty" yaml:"hsts_subdomains,omitempty"`
	AdvancedConfig        string             `json:"advanced_config,omitempty" yaml:"advanced_config,omitempty"`
	Locations             []StaticLocation   `json:"locations,omitempty" yaml:"locations,omitempty"`
	Middlewares           *middleware.Config `json:"middlewares,omitempty" yaml:"middlewares,omitempty"`
	AccessListID          string             `json:"access_list_id,omitempty" yaml:"access_list_id,omitempty"`
	Enabled               *bool              `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	SourceFile            string             `json:"source_file,omitempty" yaml:"-"`
}

// StaticStream defines a declarative Layer 4 TCP/UDP stream mapping.
type StaticStream struct {
	IncomingPort   int    `json:"incoming_port" yaml:"incoming_port"`
	ForwardingHost string `json:"forwarding_host" yaml:"forwarding_host"`
	ForwardingPort int    `json:"forwarding_port" yaml:"forwarding_port"`
	TCPForwarding  *bool  `json:"tcp_forwarding,omitempty" yaml:"tcp_forwarding,omitempty"`
	UDPForwarding  *bool  `json:"udp_forwarding,omitempty" yaml:"udp_forwarding,omitempty"`
	Enabled        *bool  `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	SourceFile     string `json:"source_file,omitempty" yaml:"-"`
}

// RouteManifest represents the top-level structure of a routes.yaml file.
type RouteManifest struct {
	Version    string            `json:"version,omitempty" yaml:"version,omitempty"`
	ProxyHosts []StaticProxyHost `json:"proxy_hosts" yaml:"proxy_hosts"`
	Streams    []StaticStream    `json:"streams,omitempty" yaml:"streams,omitempty"`
}
