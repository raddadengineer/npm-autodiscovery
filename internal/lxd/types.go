package lxd

import (
	"encoding/json"
	"time"
)

// Instance represents an LXD or Incus container or VM instance.
type Instance struct {
	Name           string            `json:"name"`
	Status         string            `json:"status"`      // e.g. "Running", "Stopped"
	StatusCode     int               `json:"status_code"` // 103 = Running
	Type           string            `json:"type"`        // "container" or "virtual-machine"
	Config         map[string]string `json:"config"`
	ExpandedConfig map[string]string `json:"expanded_config,omitempty"`
	State          *InstanceState    `json:"state,omitempty"`
}

// InstanceState represents active runtime state returned by /state.
type InstanceState struct {
	Status     string                           `json:"status"`
	StatusCode int                              `json:"status_code"`
	Network    map[string]NetworkInterfaceState `json:"network"`
}

// NetworkInterfaceState holds active IP addresses and status for an interface.
type NetworkInterfaceState struct {
	Addresses []Address `json:"addresses"`
	Hwaddr    string    `json:"hwaddr,omitempty"`
	State     string    `json:"state"` // "up", "down"
	Type      string    `json:"type"`  // "broadcast", "loopback", etc.
}

// Address represents an IP address configured on an interface.
type Address struct {
	Family  string `json:"family"`  // "inet" (IPv4), "inet6" (IPv6)
	Address string `json:"address"` // e.g. "192.168.1.105"
	Netmask string `json:"netmask"` // e.g. "24"
	Scope   string `json:"scope"`   // "global", "link", "local"
}

// ServerInfo holds hypervisor daemon details returned by /1.0.
type ServerInfo struct {
	APIVersion  string            `json:"api_version"`
	ServerName  string            `json:"server_name"`
	Environment ServerEnvironment `json:"environment"`
}

// ServerEnvironment holds engine branding ("lxd" or "incus") and version.
type ServerEnvironment struct {
	Server        string `json:"server"`         // "lxd" or "incus"
	ServerVersion string `json:"server_version"` // e.g. "5.21" or "6.0"
	Kernel        string `json:"kernel"`
	OSName        string `json:"os_name"`
}

// APIResponse wraps the LXD/Incus standard JSON envelope.
type APIResponse struct {
	Type       string          `json:"type"` // "sync", "async", "error"
	Status     string          `json:"status"`
	StatusCode int             `json:"status_code"`
	Metadata   json.RawMessage `json:"metadata"`
	Error      string          `json:"error,omitempty"`
}

// LifecycleEvent represents a real-time event from /1.0/events?type=lifecycle.
type LifecycleEvent struct {
	Type      string        `json:"type"`
	Timestamp string        `json:"timestamp"`
	Metadata  EventMetadata `json:"metadata"`
}

// EventMetadata describes the lifecycle change (action, container name, context).
type EventMetadata struct {
	Action  string                 `json:"action"` // "instance-started", "instance-stopped", "instance-updated", etc.
	Source  string                 `json:"source"` // e.g. "/1.0/instances/my-container"
	Context map[string]interface{} `json:"context,omitempty"`
}

// LXDRoute represents an auto-discovered proxy route for an LXD/Incus instance.
type LXDRoute struct {
	Name                  string        `json:"name"`
	DomainNames           []string      `json:"domain_names"`
	Path                  string        `json:"path"`
	ForwardScheme         string        `json:"forward_scheme"`
	ForwardHost           string        `json:"forward_host"`
	ForwardPort           int           `json:"forward_port"`
	Interface             string        `json:"interface"`
	SSLEnabled            bool          `json:"ssl_enabled"`
	SSLForced             bool          `json:"ssl_forced"`
	CertificateID         interface{}   `json:"certificate_id,omitempty"`
	AllowWebsocketUpgrade bool          `json:"allow_websocket_upgrade"`
	BlockExploits         bool          `json:"block_exploits"`
	CachingEnabled        bool          `json:"caching_enabled"`
	HTTP2Support          bool          `json:"http2_support"`
	HSTSEnabled           bool          `json:"hsts_enabled"`
	HSTSSubdomains        bool          `json:"hsts_subdomains"`
	AccessListID          string        `json:"access_list_id,omitempty"`
	AdvancedConfig        string        `json:"advanced_config,omitempty"`
	Locations             []LXDLocation `json:"locations,omitempty"`
	DiscoveredAt          time.Time     `json:"discovered_at"`
}

// LXDLocation represents subpath custom location routing.
type LXDLocation struct {
	Path           string `json:"path"`
	ForwardScheme  string `json:"forward_scheme"`
	ForwardHost    string `json:"forward_host"`
	ForwardPort    int    `json:"forward_port"`
	AdvancedConfig string `json:"advanced_config,omitempty"`
}

// LXDStream represents a Layer 4 TCP/UDP stream proxy discovered from LXD/Incus.
type LXDStream struct {
	Name           string `json:"name"`
	IncomingPort   int    `json:"incoming_port"`
	ForwardingHost string `json:"forwarding_host"`
	ForwardingPort int    `json:"forwarding_port"`
	TCPForwarding  bool   `json:"tcp_forwarding"`
	UDPForwarding  bool   `json:"udp_forwarding"`
	Interface      string `json:"interface"`
}
