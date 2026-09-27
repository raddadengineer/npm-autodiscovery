package pve

import (
	"encoding/json"
	"time"
)

// LXCContainerSummary represents an LXC container returned by /api2/json/nodes/{node}/lxc.
type LXCContainerSummary struct {
	VMID    int     `json:"vmid"`
	Name    string  `json:"name"`
	Status  string  `json:"status"` // "running", "stopped", etc.
	Type    string  `json:"type"`   // "lxc"
	Tags    string  `json:"tags,omitempty"`
	CPUs    int     `json:"cpus,omitempty"`
	MaxMem  int64   `json:"maxmem,omitempty"`
	Mem     int64   `json:"mem,omitempty"`
	MaxDisk int64   `json:"maxdisk,omitempty"`
	Uptime  int64   `json:"uptime,omitempty"`
	Node    string  `json:"node,omitempty"`
}

// LXCConfig represents the detailed configuration of an LXC container from /config.
type LXCConfig struct {
	Description  string                 `json:"description,omitempty"` // Container Notes / Description field
	Tags         string                 `json:"tags,omitempty"`        // Semicolon/comma separated tags
	Hostname     string                 `json:"hostname,omitempty"`
	Nameserver   string                 `json:"nameserver,omitempty"`
	Searchdomain string                 `json:"searchdomain,omitempty"`
	Net0         string                 `json:"net0,omitempty"`
	Net1         string                 `json:"net1,omitempty"`
	Net2         string                 `json:"net2,omitempty"`
	Net3         string                 `json:"net3,omitempty"`
	Raw          map[string]interface{} `json:"-"`
}

// NetworkInterface represents an interface returned by /interfaces.
type NetworkInterface struct {
	Name   string `json:"name"`
	HWAddr string `json:"hwaddr,omitempty"`
	Inet   string `json:"inet,omitempty"`  // IPv4 with CIDR e.g. "192.168.1.101/24"
	Inet6  string `json:"inet6,omitempty"` // IPv6 with CIDR
}

// NodeSummary represents a cluster node from /api2/json/nodes.
type NodeSummary struct {
	Node   string `json:"node"`
	Status string `json:"status"` // "online", "offline"
	Type   string `json:"type"`
	SSL    string `json:"ssl_fingerprint,omitempty"`
}

// APIResponse wraps the Proxmox VE REST standard envelope {"data": ...}.
type APIResponse struct {
	Data json.RawMessage `json:"data"`
}

// VersionInfo represents /api2/json/version response.
type VersionInfo struct {
	Version string `json:"version"`
	Release string `json:"release"`
	RepoID  string `json:"repoid"`
}

// PVERoute represents a fully resolved reverse proxy host discovered from Proxmox LXC.
type PVERoute struct {
	VMID                  int            `json:"vmid"`
	Node                  string         `json:"node"`
	ContainerName         string         `json:"container_name"`
	DomainNames           []string       `json:"domain_names"`
	Path                  string         `json:"path"`
	ForwardScheme         string         `json:"forward_scheme"`
	ForwardHost           string         `json:"forward_host"`
	ForwardPort           int            `json:"forward_port"`
	Interface             string         `json:"interface"`
	SSLEnabled            bool           `json:"ssl_enabled"`
	SSLForced             bool           `json:"ssl_forced"`
	CertificateID         interface{}    `json:"certificate_id,omitempty"`
	AllowWebsocketUpgrade bool           `json:"allow_websocket_upgrade"`
	BlockExploits         bool           `json:"block_exploits"`
	CachingEnabled        bool           `json:"caching_enabled"`
	HTTP2Support          bool           `json:"http2_support"`
	HSTSEnabled           bool           `json:"hsts_enabled"`
	HSTSSubdomains        bool           `json:"hsts_subdomains"`
	AccessListID          string         `json:"access_list_id,omitempty"`
	AdvancedConfig        string         `json:"advanced_config,omitempty"`
	Locations             []PVELocation  `json:"locations,omitempty"`
	Tags                  string         `json:"tags,omitempty"`
	DiscoveryChannel      string         `json:"discovery_channel"` // "tags", "notes", "both"
	DiscoveredAt          time.Time      `json:"discovered_at"`
}

// PVELocation represents subpath routing for Proxmox LXC.
type PVELocation struct {
	Path           string `json:"path"`
	ForwardScheme  string `json:"forward_scheme"`
	ForwardHost    string `json:"forward_host"`
	ForwardPort    int    `json:"forward_port"`
	AdvancedConfig string `json:"advanced_config,omitempty"`
}

// PVEStream represents a Layer 4 TCP/UDP stream proxy discovered from Proxmox LXC.
type PVEStream struct {
	VMID           int    `json:"vmid"`
	Node           string `json:"node"`
	ContainerName  string `json:"container_name"`
	IncomingPort   int    `json:"incoming_port"`
	ForwardingHost string `json:"forwarding_host"`
	ForwardingPort int    `json:"forwarding_port"`
	TCPForwarding  bool   `json:"tcp_forwarding"`
	UDPForwarding  bool   `json:"udp_forwarding"`
	Interface      string `json:"interface"`
}
