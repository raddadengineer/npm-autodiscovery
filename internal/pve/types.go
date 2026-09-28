package pve

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// LXCContainerSummary represents an LXC container returned by /api2/json/nodes/{node}/lxc or /api2/json/cluster/resources.
type LXCContainerSummary struct {
	VMID    int     `json:"vmid"`
	Name    string  `json:"name"`
	Status  string  `json:"status"` // "running", "stopped", etc.
	Type    string  `json:"type"`   // "lxc"
	Tags    string  `json:"tags,omitempty"`
	CPUs    float64 `json:"cpus,omitempty"`
	MaxMem  int64   `json:"maxmem,omitempty"`
	Mem     int64   `json:"mem,omitempty"`
	MaxDisk int64   `json:"maxdisk,omitempty"`
	Uptime  int64   `json:"uptime,omitempty"`
	Node    string  `json:"node,omitempty"`
	Notes   string  `json:"notes,omitempty"`
	IP      string  `json:"ip,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling to handle variable Proxmox VE types safely.
func (s *LXCContainerSummary) UnmarshalJSON(data []byte) error {
	var raw struct {
		VMID    interface{} `json:"vmid"`
		Name    string      `json:"name"`
		Status  string      `json:"status"`
		Type    string      `json:"type"`
		Tags    interface{} `json:"tags"`
		CPUs    interface{} `json:"cpus"`
		MaxMem  interface{} `json:"maxmem"`
		Mem     interface{} `json:"mem"`
		MaxDisk interface{} `json:"maxdisk"`
		Uptime  interface{} `json:"uptime"`
		Node    string      `json:"node"`
		Notes   string      `json:"notes"`
		IP      string      `json:"ip"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	s.Name = raw.Name
	s.Status = raw.Status
	s.Type = raw.Type
	s.Node = raw.Node
	s.Notes = raw.Notes
	s.IP = raw.IP

	switch v := raw.VMID.(type) {
	case float64:
		s.VMID = int(v)
	case int:
		s.VMID = v
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			s.VMID = n
		}
	}

	switch v := raw.Tags.(type) {
	case string:
		s.Tags = v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if str, ok := item.(string); ok {
				parts = append(parts, str)
			}
		}
		s.Tags = strings.Join(parts, ",")
	}

	switch v := raw.CPUs.(type) {
	case float64:
		s.CPUs = v
	case int:
		s.CPUs = float64(v)
	}

	parseInt64 := func(val interface{}) int64 {
		switch v := val.(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		case int64:
			return v
		}
		return 0
	}

	s.MaxMem = parseInt64(raw.MaxMem)
	s.Mem = parseInt64(raw.Mem)
	s.MaxDisk = parseInt64(raw.MaxDisk)
	s.Uptime = parseInt64(raw.Uptime)

	return nil
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
