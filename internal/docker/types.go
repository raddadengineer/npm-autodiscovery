package docker

// Port describes a port mapping or exposed port in Docker.
type Port struct {
	IP          string `json:"IP,omitempty"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort,omitempty"`
	Type        string `json:"Type"`
}

// NetworkEndpointSettings holds IP and network configuration for a specific network.
type NetworkEndpointSettings struct {
	IPAddress   string   `json:"IPAddress"`
	IPPrefixLen int      `json:"IPPrefixLen"`
	Gateway     string   `json:"Gateway"`
	NetworkID   string   `json:"NetworkID"`
	Aliases     []string `json:"Aliases,omitempty"`
}

// PortBinding describes published host ports in container inspection.
type PortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

// NetworkSettings holds container network configurations.
type NetworkSettings struct {
	IPAddress string                             `json:"IPAddress"`
	Gateway   string                             `json:"Gateway"`
	Ports     map[string][]PortBinding           `json:"Ports,omitempty"`
	Networks  map[string]NetworkEndpointSettings `json:"Networks"`
}

// ContainerSummary represents a container returned by /containers/json.
type ContainerSummary struct {
	ID              string            `json:"Id"`
	Names           []string          `json:"Names"`
	Image           string            `json:"Image"`
	Command         string            `json:"Command"`
	Created         int64             `json:"Created"`
	State           string            `json:"State"`
	Status          string            `json:"Status"`
	Labels          map[string]string `json:"Labels"`
	Ports           []Port            `json:"Ports"`
	NetworkSettings NetworkSettings   `json:"NetworkSettings"`
}

// ContainerConfig holds container configuration details.
type ContainerConfig struct {
	Hostname     string                 `json:"Hostname"`
	Domainname   string                 `json:"Domainname"`
	Labels       map[string]string      `json:"Labels"`
	ExposedPorts map[string]interface{} `json:"ExposedPorts"`
	Image        string                 `json:"Image"`
}

// ContainerInspect represents full container details from /containers/{id}/json.
type ContainerInspect struct {
	ID              string           `json:"Id"`
	Created         string           `json:"Created"`
	Name            string           `json:"Name"`
	State           ContainerState   `json:"State"`
	Config          ContainerConfig  `json:"Config"`
	NetworkSettings NetworkSettings  `json:"NetworkSettings"`
}

// ContainerState represents container execution status.
type ContainerState struct {
	Status   string `json:"Status"`
	Running  bool   `json:"Running"`
	Paused   bool   `json:"Paused"`
	Restarting bool `json:"Restarting"`
	OOMKilled bool  `json:"OOMKilled"`
	Dead     bool   `json:"Dead"`
	Pid      int    `json:"Pid"`
	ExitCode int    `json:"ExitCode"`
	Error    string `json:"Error"`
}

// Actor describes the target of a Docker event.
type Actor struct {
	ID         string            `json:"ID"`
	Attributes map[string]string `json:"Attributes"`
}

// Event represents a Docker engine event from /events.
type Event struct {
	Type     string `json:"Type"`
	Action   string `json:"Action"`
	Actor    Actor  `json:"Actor"`
	Scope    string `json:"scope"`
	Time     int64  `json:"time"`
	TimeNano int64  `json:"timeNano"`
}

// VersionResponse represents Docker engine version information.
type VersionResponse struct {
	Version       string `json:"Version"`
	APIVersion    string `json:"ApiVersion"`
	MinAPIVersion string `json:"MinAPIVersion"`
	GitCommit     string `json:"GitCommit"`
	GoVersion     string `json:"GoVersion"`
	Os            string `json:"Os"`
	Arch          string `json:"Arch"`
	KernelVersion string `json:"KernelVersion"`
}
