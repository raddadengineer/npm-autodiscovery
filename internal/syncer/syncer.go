package syncer

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/raddadengineer/npm-autodiscovery/internal/config"
	"github.com/raddadengineer/npm-autodiscovery/internal/docker"
	"github.com/raddadengineer/npm-autodiscovery/internal/iac"
	"github.com/raddadengineer/npm-autodiscovery/internal/lxd"
	"github.com/raddadengineer/npm-autodiscovery/internal/metrics"
	"github.com/raddadengineer/npm-autodiscovery/internal/npm"
	"github.com/raddadengineer/npm-autodiscovery/internal/pve"
	"github.com/raddadengineer/npm-autodiscovery/internal/upstream"
)

const (
	ManagedByTag   = "npm-autodiscovery"
	HeaderComment  = "# Managed by NPM-AutoDiscovery"
	MaxLogsHistory = 500
)

// LogLevel represents event severity.
type LogLevel string

const (
	LevelInfo    LogLevel = "info"
	LevelSuccess LogLevel = "success"
	LevelWarn    LogLevel = "warn"
	LevelError   LogLevel = "error"
)

// LogEvent represents a structured log entry sent to the live UI event log.
type LogEvent struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     LogLevel  `json:"level"`
	Category  string    `json:"category"` // "discovery", "sync", "docker", "npm", "system", "health", "pve", "lxd", "cluster"
	Message   string    `json:"message"`
	Details   string    `json:"details,omitempty"`
	NodeID    string    `json:"node_id,omitempty"`
}

// ManagedProxy tracks a single auto-discovered proxy host state.
type ManagedProxy struct {
	NPMHostID        int                     `json:"npm_host_id"`
	HostID           string                  `json:"host_id"`
	ContainerID      string                  `json:"container_id"`
	ContainerName    string                  `json:"container_name"`
	Image            string                  `json:"image"`
	DomainNames      []string                `json:"domain_names"`
	ForwardScheme    string                  `json:"forward_scheme"`
	ForwardHost      string                  `json:"forward_host"`
	ForwardPort      int                     `json:"forward_port"`
	ResolutionMethod string                  `json:"resolution_method"`
	SSLEnabled       bool                    `json:"ssl_enabled"`
	SSLForced        bool                    `json:"ssl_forced"`
	CertificateID    any                     `json:"certificate_id"`
	Websocket        bool                    `json:"websocket"`
	BlockExploits    bool                    `json:"block_exploits"`
	HealthStatus     string                  `json:"health_status,omitempty"`
	Locations        []npm.ProxyHostLocation `json:"locations,omitempty"`
	Source           string                  `json:"source"` // "docker", "iac", "pve", "lxd"
	SourceFile       string                  `json:"source_file,omitempty"`
	Status           string                  `json:"status"` // "active", "syncing", "error"
	LastSynced       time.Time               `json:"last_synced"`
}

// ManagedStream tracks an active Layer 4 TCP/UDP stream proxy in NPM.
type ManagedStream struct {
	NPMStreamID      int       `json:"npm_stream_id"`
	HostID           string    `json:"host_id"`
	ContainerID      string    `json:"container_id,omitempty"`
	ContainerName    string    `json:"container_name,omitempty"`
	IncomingPort     int       `json:"incoming_port"`
	ForwardingHost   string    `json:"forwarding_host"`
	ForwardingPort   int       `json:"forwarding_port"`
	TCPForwarding    bool      `json:"tcp_forwarding"`
	UDPForwarding    bool      `json:"udp_forwarding"`
	ResolutionMethod string    `json:"resolution_method"`
	Source           string    `json:"source"` // "docker", "iac", "pve", "lxd"
	SourceFile       string    `json:"source_file,omitempty"`
	Status           string    `json:"status"` // "active", "syncing", "error"
	LastSynced       time.Time `json:"last_synced"`
}

// ContainerStatusView represents running container info for the UI container explorer.
type ContainerStatusView struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	Source        string            `json:"source"` // "docker", "pve", "lxd"
	State         string            `json:"state"`
	Status        string            `json:"status"`
	HealthStatus  string            `json:"health_status,omitempty"`
	Discovered    bool              `json:"discovered"`
	ManualNPM     bool              `json:"manual_npm,omitempty"`
	IgnoredReason string            `json:"ignored_reason,omitempty"`
	Domains       []string          `json:"domains,omitempty"`
	Port          int               `json:"port,omitempty"`
	Labels        map[string]string `json:"labels"`
	NodeID        string            `json:"node_id,omitempty"`
}

// StatusOverview reports system health and counters.
type StatusOverview struct {
	HostID            string    `json:"host_id"`
	HostIP            string    `json:"host_ip,omitempty"`
	DockerConnected   bool      `json:"docker_connected"`
	DockerSocket      string    `json:"docker_socket"`
	DockerVersion     string    `json:"docker_version"`
	NPMConnected      bool      `json:"npm_connected"`
	NPMURL            string    `json:"npm_url"`
	ActiveProxies     int       `json:"active_proxies"`
	ActiveStreams     int       `json:"active_streams"`
	RunningContainers int       `json:"running_containers"`
	TotalEvents       int64     `json:"total_events"`
	LastFullSync      time.Time `json:"last_full_sync"`
	Uptime            int64     `json:"uptime_seconds"`

	// Hypervisors: Proxmox VE (Phase 3.1)
	PVEEnabled           bool   `json:"pve_enabled"`
	PVEConnected         bool   `json:"pve_connected"`
	PVEURL               string `json:"pve_url,omitempty"`
	PVENode              string `json:"pve_node,omitempty"`
	PVEVersion           string `json:"pve_version,omitempty"`
	PVERunningContainers int    `json:"pve_running_containers"`

	// Hypervisors: Canonical LXD / Incus (Phase 3.2)
	LXDEnabled           bool   `json:"lxd_enabled"`
	LXDConnected         bool   `json:"lxd_connected"`
	LXDSocket            string `json:"lxd_socket,omitempty"`
	LXDServerType        string `json:"lxd_server_type,omitempty"`
	LXDVersion           string `json:"lxd_version,omitempty"`
	LXDRunningContainers int    `json:"lxd_running_containers"`

	// Multi-Node Cluster Overview (Central Controller)
	ClusterNodesCount  int `json:"cluster_nodes_count,omitempty"`
	ClusterOnlineNodes int `json:"cluster_online_nodes,omitempty"`
}

// PendingHealthCheck tracks containers in warming up/starting state awaiting healthy before provisioning.
type PendingHealthCheck struct {
	ContainerID      string
	ContainerName    string
	Inspect          *docker.ContainerInspect
	ProxyCfg         *ContainerProxyConfig
	ResolutionMethod string
	CreatedAt        time.Time
	Timer            *time.Timer
}

// Syncer is the primary coordination engine bridging Docker, Hypervisors, and NPM.
type Syncer struct {
	cfg          *config.Config
	dockerClient *docker.Client
	npmClient    *npm.Client
	pveClient    *pve.Client
	pveClients   map[string]*pve.Client // Key: config ID -> client
	pveConfigs   map[string]PVEConfig   // Key: config ID -> config
	lxdClient    *lxd.Client

	mu             sync.RWMutex
	trackedProxies map[string]*ManagedProxy       // Key: primary domain or container ID
	manualProxies  map[int]*ManagedProxy          // Key: NPM host ID (configured via NPM portal)
	trackedStreams map[int]*ManagedStream         // Key: incoming_port
	pendingHealth  map[string]*PendingHealthCheck // Key: container ID
	pveContainers  []pve.LXCContainerSummary
	lxdInstances   []lxd.Instance
	logEvents      []LogEvent
	logSubscribers map[chan LogEvent]struct{}
	logSeq         int64
	startTime      time.Time
	lastFullSync   time.Time
	totalEvents    int64
	dockerVersion  string

	syncTrigger       chan struct{}
	workerPushTrigger chan struct{}
	clusterRegistry   *ClusterRegistry

	certCacheMutex   sync.RWMutex
	cachedCerts      []npm.Certificate
	certCacheExpires time.Time
}

// NewSyncer instantiates the syncer with dependencies.
func NewSyncer(cfg *config.Config, dClient *docker.Client, nClient *npm.Client) *Syncer {
	return &Syncer{
		cfg:               cfg,
		dockerClient:      dClient,
		npmClient:         nClient,
		pveClients:        make(map[string]*pve.Client),
		pveConfigs:        make(map[string]PVEConfig),
		trackedProxies:    make(map[string]*ManagedProxy),
		manualProxies:     make(map[int]*ManagedProxy),
		trackedStreams:    make(map[int]*ManagedStream),
		pendingHealth:     make(map[string]*PendingHealthCheck),
		logEvents:         make([]LogEvent, 0, MaxLogsHistory),
		logSubscribers:    make(map[chan LogEvent]struct{}),
		startTime:         time.Now(),
		syncTrigger:       make(chan struct{}, 1),
		workerPushTrigger: make(chan struct{}, 1),
		clusterRegistry:   NewClusterRegistry(),
	}
}

// NewSyncerWithProviders instantiates the syncer with hypervisor clients.
func NewSyncerWithProviders(cfg *config.Config, dClient *docker.Client, nClient *npm.Client, pveClient *pve.Client, lxdClient *lxd.Client) *Syncer {
	s := NewSyncer(cfg, dClient, nClient)
	s.pveClient = pveClient
	if pveClient != nil {
		s.pveClients["default"] = pveClient
	}
	s.lxdClient = lxdClient
	return s
}

// SetPVEClient sets the Proxmox VE provider client.
func (s *Syncer) SetPVEClient(client *pve.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pveClient = client
	if client != nil {
		s.pveClients["default"] = client
	} else {
		delete(s.pveClients, "default")
	}
}

// SetLXDClient sets the Canonical LXD / Incus provider client.
func (s *Syncer) SetLXDClient(client *lxd.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lxdClient = client
}

// Start launches the background synchronization, Docker event listener, and periodic scanner.
func (s *Syncer) Start(ctx context.Context) {
	s.EmitLog(LevelInfo, "system", "NPM Auto-Discovery agent starting up", fmt.Sprintf("Poll interval: %s, Strategy: %s", s.cfg.PollInterval, s.cfg.ForwardHostStrategy))

	// Initial Docker check & version fetch
	if s.dockerClient != nil {
		if v, err := s.dockerClient.GetVersion(ctx); err == nil {
			s.mu.Lock()
			s.dockerVersion = fmt.Sprintf("%s (API %s)", v.Version, v.APIVersion)
			s.mu.Unlock()
			s.EmitLog(LevelSuccess, "docker", fmt.Sprintf("Connected to Docker daemon %s", s.dockerVersion), s.dockerClient.SocketPath())
		} else {
			s.EmitLog(LevelWarn, "docker", "Could not query Docker version initially", err.Error())
		}
	}

	// Initial Proxmox VE check
	if s.pveClient != nil {
		if err := s.pveClient.Ping(ctx); err == nil {
			s.EmitLog(LevelSuccess, "pve", fmt.Sprintf("Connected to Proxmox VE API %s", s.pveClient.Version()), s.pveClient.BaseURL())
		} else {
			s.EmitLog(LevelWarn, "pve", "Could not connect to Proxmox VE API initially (will retry)", err.Error())
		}
	} else if s.clusterRegistry != nil {
		localID := s.cfg.HostID
		if localID == "" {
			localID = "controller-main"
		}
		if pveCfgs := s.clusterRegistry.GetNodePVEConfigs(localID); len(pveCfgs) > 0 {
			_ = s.ApplyDynamicPVEConfigs(pveCfgs)
		} else if pveCfg := s.clusterRegistry.GetNodePVEConfig(localID); pveCfg != nil && pveCfg.Enabled {
			_ = s.ApplyDynamicPVEConfig(*pveCfg)
		}
	}

	// Initial LXD / Incus check & event listener
	if s.lxdClient != nil {
		if err := s.lxdClient.Ping(ctx); err == nil {
			s.EmitLog(LevelSuccess, "lxd", fmt.Sprintf("Connected to %s daemon %s", strings.ToUpper(s.lxdClient.ServerType()), s.lxdClient.ServerVersion()), s.lxdClient.SocketPath())
			go s.listenLXDEvents(ctx)
		} else {
			s.EmitLog(LevelWarn, "lxd", "Could not connect to LXD/Incus unix socket initially (will retry)", err.Error())
		}
	}

	// Initial NPM authentication check
	if s.npmClient != nil {
		if err := s.npmClient.Authenticate(ctx); err == nil {
			s.EmitLog(LevelSuccess, "npm", "Authenticated successfully with Nginx Proxy Manager", s.cfg.NPMURL)
		} else {
			s.EmitLog(LevelError, "npm", "Initial authentication with NPM failed (will retry)", err.Error())
		}
	}

	// Run initial full sync
	s.TriggerSync()

	// 1. Docker Event Stream Goroutine
	if s.dockerClient != nil {
		go s.listenDockerEvents(ctx)
	}

	// 2. Periodic Scan & Trigger Goroutine
	go s.runPeriodicSync(ctx)

	// 3. Multi-Node Cluster Telemetry Pusher (if running in Remote Worker mode)
	if s.cfg.MainNodeURL != "" {
		go s.startWorkerReporter(ctx)
	}
}

// listenLXDEvents listens for real-time lifecycle events from Canonical LXD / Incus.
func (s *Syncer) listenLXDEvents(ctx context.Context) {
	if s.lxdClient == nil {
		return
	}
	eventChan, errChan := s.lxdClient.StreamEvents(ctx)
	s.EmitLog(LevelInfo, "lxd", fmt.Sprintf("Listening for %s lifecycle events", s.lxdClient.ServerType()), "")

	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-errChan:
			if !ok {
				return
			}
			s.EmitLog(LevelWarn, "lxd", "LXD/Incus event stream warning", err.Error())
		case ev, ok := <-eventChan:
			if !ok {
				return
			}
			s.mu.Lock()
			s.totalEvents++
			s.mu.Unlock()
			s.EmitLog(LevelInfo, "lxd", fmt.Sprintf("LXD Lifecycle event: %s (%s)", ev.Metadata.Action, ev.Metadata.Source), "")
			s.TriggerSync()
		}
	}
}

// TriggerSync signals an immediate full sync.
func (s *Syncer) TriggerSync() {
	select {
	case s.syncTrigger <- struct{}{}:
	default:
	}
	s.triggerWorkerPush()
}

// runPeriodicSync loops on the poll timer and manual sync signals.
func (s *Syncer) runPeriodicSync(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.EmitLog(LevelInfo, "system", "Periodic sync engine stopped", "")
			return

		case <-s.syncTrigger:
			s.performFullSync(ctx, "manual/startup")

		case <-ticker.C:
			s.performFullSync(ctx, "periodic")
		}
	}
}

// listenDockerEvents listens for live Docker container events.
func (s *Syncer) listenDockerEvents(ctx context.Context) {
	eventChan, errChan := s.dockerClient.StreamEvents(ctx)
	s.EmitLog(LevelInfo, "docker", "Listening for Docker container lifecycle events", "")

	for {
		select {
		case <-ctx.Done():
			return

		case err, ok := <-errChan:
			if !ok {
				return
			}
			s.EmitLog(LevelWarn, "docker", "Docker event stream warning", err.Error())

		case event, ok := <-eventChan:
			if !ok {
				return
			}

			s.mu.Lock()
			s.totalEvents++
			s.mu.Unlock()

			s.handleDockerEvent(ctx, event)
		}
	}
}

// handleDockerEvent routes events to container creation/update or deletion.
func (s *Syncer) handleDockerEvent(ctx context.Context, ev docker.Event) {
	containerID := ev.Actor.ID
	containerName := ev.Actor.Attributes["name"]
	if containerName == "" {
		containerName = containerID[:min(12, len(containerID))]
	}

	metrics.IncEvents(ev.Action, "success")

	switch {
	case ev.Action == "start":
		s.EmitLog(LevelInfo, "docker", fmt.Sprintf("Container started: '%s'", containerName), fmt.Sprintf("ID: %s", containerID[:min(12, len(containerID))]))
		s.syncContainerByID(ctx, containerID)
		s.performFullSync(ctx, "container-start")

	case strings.HasPrefix(ev.Action, "health_status"):
		status := strings.TrimSpace(strings.TrimPrefix(ev.Action, "health_status:"))
		s.handleHealthStatusChange(ctx, containerID, containerName, status)

	case ev.Action == "die" || ev.Action == "kill" || ev.Action == "stop" || ev.Action == "destroy":
		s.cancelPendingHealthCheck(containerID)
		s.EmitLog(LevelInfo, "docker", fmt.Sprintf("Container stopped/destroyed: '%s' (action: %s)", containerName, ev.Action), fmt.Sprintf("ID: %s", containerID[:min(12, len(containerID))]))
		s.cleanupContainerProxies(ctx, containerID, containerName)
		s.performFullSync(ctx, "container-stop")
	}
}

// syncContainerByID inspects a container and provisions/updates its proxy in NPM.
func (s *Syncer) syncContainerByID(ctx context.Context, containerID string) {
	inspect, err := s.dockerClient.InspectContainer(ctx, containerID)
	if err != nil {
		s.EmitLog(LevelWarn, "docker", fmt.Sprintf("Failed to inspect container %s", containerID[:min(12, len(containerID))]), err.Error())
		return
	}

	// Only process running containers
	if !inspect.State.Running {
		return
	}

	proxyCfg, shouldProxy, err := ParseContainerLabels(inspect, s.cfg)
	if err != nil {
		s.EmitLog(LevelWarn, "discovery", fmt.Sprintf("Invalid labels on container '%s'", inspect.Name), err.Error())
		return
	}

	if !shouldProxy || proxyCfg == nil {
		// Not configured for discovery
		return
	}

	// Resolve forward host/IP
	targetHost, resMethod, err := ResolveForwardHost(inspect, proxyCfg, s.cfg)
	if err != nil {
		s.EmitLog(LevelError, "discovery", fmt.Sprintf("Network resolution failed for container '%s'", inspect.Name), err.Error())
		return
	}
	proxyCfg.ForwardHost = targetHost

	// Zero-502 HealthCheck Check (Phase 1)
	if s.shouldHoldForHealthCheck(inspect, proxyCfg) {
		s.queuePendingHealthCheck(ctx, inspect, proxyCfg, resMethod)
		return
	}

	// If container was in pendingHealth and is now healthy, clear pending
	s.cancelPendingHealthCheck(containerID)

	s.EmitLog(LevelInfo, "discovery", fmt.Sprintf("Discovered container '%s' -> %s://%s:%d for domains %v",
		proxyCfg.ContainerName, proxyCfg.ForwardScheme, proxyCfg.ForwardHost, proxyCfg.ForwardPort, proxyCfg.DomainNames),
		fmt.Sprintf("Resolution: %s", resMethod))

	s.reconcileProxyInNPM(ctx, inspect, proxyCfg, resMethod)
}

// shouldHoldForHealthCheck returns true if a container has a Docker healthcheck and is still warming up.
func (s *Syncer) shouldHoldForHealthCheck(inspect *docker.ContainerInspect, proxyCfg *ContainerProxyConfig) bool {
	if !proxyCfg.HealthCheck.Enabled {
		return false
	}
	// If no Docker healthcheck is defined in Dockerfile/compose, provision directly
	if inspect.State.Health == nil {
		return false
	}
	// If already healthy, do not hold
	if inspect.State.Health.Status == "healthy" {
		return false
	}
	return true
}

// queuePendingHealthCheck places a warming-up container into the pending health queue.
func (s *Syncer) queuePendingHealthCheck(ctx context.Context, inspect *docker.ContainerInspect, proxyCfg *ContainerProxyConfig, resMethod string) {
	s.mu.Lock()
	containerID := inspect.ID
	containerName := proxyCfg.ContainerName

	if existing, ok := s.pendingHealth[containerID]; ok && existing.Timer != nil {
		existing.Timer.Stop()
	}

	healthStatus := "starting"
	if inspect.State.Health != nil && inspect.State.Health.Status != "" {
		healthStatus = inspect.State.Health.Status
	}

	timeoutDur := proxyCfg.HealthCheck.Timeout
	if timeoutDur <= 0 {
		timeoutDur = 60 * time.Second
	}

	pending := &PendingHealthCheck{
		ContainerID:      containerID,
		ContainerName:    containerName,
		Inspect:          inspect,
		ProxyCfg:         proxyCfg,
		ResolutionMethod: resMethod,
		CreatedAt:        time.Now(),
	}

	pending.Timer = time.AfterFunc(timeoutDur, func() {
		s.handleHealthCheckTimeout(context.Background(), containerID)
	})

	s.pendingHealth[containerID] = pending
	s.mu.Unlock()

	s.EmitLog(LevelInfo, "health", fmt.Sprintf("Zero-502: Container '%s' warming up (health: %s). Ingress queued (timeout: %s, fallback: %s)",
		containerName, healthStatus, timeoutDur, proxyCfg.HealthCheck.FallbackAction),
		fmt.Sprintf("Domains: %v -> :%d", proxyCfg.DomainNames, proxyCfg.ForwardPort))
}

// handleHealthCheckTimeout handles container healthcheck timeout.
func (s *Syncer) handleHealthCheckTimeout(ctx context.Context, containerID string) {
	s.mu.Lock()
	pending, ok := s.pendingHealth[containerID]
	if !ok {
		s.mu.Unlock()
		return
	}
	delete(s.pendingHealth, containerID)
	s.mu.Unlock()

	containerName := pending.ContainerName
	fallback := pending.ProxyCfg.HealthCheck.FallbackAction
	timeoutDur := pending.ProxyCfg.HealthCheck.Timeout

	s.EmitLog(LevelWarn, "health", fmt.Sprintf("Zero-502: Container '%s' exceeded healthcheck timeout (%s). Triggering fallback '%s'.",
		containerName, timeoutDur, fallback), "")

	switch fallback {
	case "disable":
		s.EmitLog(LevelWarn, "health", fmt.Sprintf("Ingress for container '%s' disabled due to failed warmup.", containerName), "")
		s.cleanupContainerProxies(ctx, containerID, containerName)

	case "keep":
		s.EmitLog(LevelWarn, "health", fmt.Sprintf("Provisioning proxy host for '%s' despite incomplete warmup (fallback: keep).", containerName), "")
		s.reconcileProxyInNPM(ctx, pending.Inspect, pending.ProxyCfg, pending.ResolutionMethod)

	case "redirect":
		if pending.ProxyCfg.HealthCheck.RedirectTarget != "" {
			target := pending.ProxyCfg.HealthCheck.RedirectTarget
			s.EmitLog(LevelWarn, "health", fmt.Sprintf("Rerouting proxy for '%s' to fallback redirect target '%s'.", containerName, target), "")
			pending.ProxyCfg.ForwardHost = target
			s.reconcileProxyInNPM(ctx, pending.Inspect, pending.ProxyCfg, "health-timeout-redirect")
		} else {
			s.EmitLog(LevelWarn, "health", fmt.Sprintf("Redirect target empty for '%s'; disabling ingress.", containerName), "")
			s.cleanupContainerProxies(ctx, containerID, containerName)
		}
	}
}

// handleHealthStatusChange handles transitions when Docker emits health status events.
func (s *Syncer) handleHealthStatusChange(ctx context.Context, containerID, containerName, status string) {
	metrics.IncHealthStatus(status)
	s.EmitLog(LevelInfo, "health", fmt.Sprintf("Docker health event for '%s': status=%s", containerName, status), fmt.Sprintf("ID: %s", containerID[:min(12, len(containerID))]))

	switch status {
	case "healthy":
		s.mu.Lock()
		pending, wasPending := s.pendingHealth[containerID]
		if wasPending {
			if pending.Timer != nil {
				pending.Timer.Stop()
			}
			delete(s.pendingHealth, containerID)
		}
		s.mu.Unlock()

		if wasPending && pending != nil {
			s.EmitLog(LevelSuccess, "health", fmt.Sprintf("Zero-502: Container '%s' became healthy! Provisioning proxy in NPM.", containerName), "")
			s.reconcileProxyInNPM(ctx, pending.Inspect, pending.ProxyCfg, pending.ResolutionMethod)
		} else {
			s.syncContainerByID(ctx, containerID)
		}

	case "unhealthy":
		s.mu.Lock()
		pending, wasPending := s.pendingHealth[containerID]
		if wasPending {
			if pending.Timer != nil {
				pending.Timer.Stop()
			}
			delete(s.pendingHealth, containerID)
		}
		s.mu.Unlock()

		inspect, err := s.dockerClient.InspectContainer(ctx, containerID)
		if err != nil {
			return
		}
		proxyCfg, shouldProxy, err := ParseContainerLabels(inspect, s.cfg)
		if err != nil || !shouldProxy || proxyCfg == nil {
			return
		}

		switch proxyCfg.HealthCheck.FallbackAction {
		case "disable":
			s.EmitLog(LevelWarn, "health", fmt.Sprintf("Container '%s' reported unhealthy! Decommissioning proxy in NPM.", containerName), "")
			s.cleanupContainerProxies(ctx, containerID, containerName)
		case "redirect":
			if proxyCfg.HealthCheck.RedirectTarget != "" {
				s.EmitLog(LevelWarn, "health", fmt.Sprintf("Container '%s' unhealthy. Routing to redirect target '%s'.", containerName, proxyCfg.HealthCheck.RedirectTarget), "")
				proxyCfg.ForwardHost = proxyCfg.HealthCheck.RedirectTarget
				s.reconcileProxyInNPM(ctx, inspect, proxyCfg, "health-unhealthy-redirect")
			}
		case "keep":
			s.EmitLog(LevelWarn, "health", fmt.Sprintf("Container '%s' unhealthy. Maintaining proxy host with warning (fallback: keep).", containerName), "")
		}
	}
}

// cancelPendingHealthCheck cancels and removes a container from the pending health queue.
func (s *Syncer) cancelPendingHealthCheck(containerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pending, ok := s.pendingHealth[containerID]; ok {
		if pending.Timer != nil {
			pending.Timer.Stop()
		}
		delete(s.pendingHealth, containerID)
	}
}

// getCertificates retrieves cached SSL certificates or queries NPM API.
func (s *Syncer) getCertificates(ctx context.Context) ([]npm.Certificate, error) {
	if s.npmClient == nil {
		return nil, nil
	}

	s.certCacheMutex.RLock()
	if time.Now().Before(s.certCacheExpires) && s.cachedCerts != nil {
		certs := s.cachedCerts
		s.certCacheMutex.RUnlock()
		return certs, nil
	}
	s.certCacheMutex.RUnlock()

	s.certCacheMutex.Lock()
	defer s.certCacheMutex.Unlock()

	if time.Now().Before(s.certCacheExpires) && s.cachedCerts != nil {
		return s.cachedCerts, nil
	}

	certs, err := s.npmClient.GetCertificates(ctx)
	if err != nil {
		return nil, err
	}

	s.cachedCerts = certs
	s.certCacheExpires = time.Now().Add(30 * time.Second)
	return certs, nil
}

// applyAutoDetectSSL checks if domains match an existing certificate in NPM and assigns it if no explicit certificate_id was set.
func (s *Syncer) applyAutoDetectSSL(ctx context.Context, domains []string, certID *interface{}, sslForced *bool, explicitCert bool, explicitForced bool) {
	if explicitCert || (*certID != 0 && *certID != nil && *certID != "") || !s.cfg.AutoDetectSSL {
		return
	}

	certs, err := s.getCertificates(ctx)
	if err != nil || len(certs) == 0 {
		return
	}

	if matchedCert := MatchCertificate(certs, domains); matchedCert != nil {
		s.EmitLog(LevelInfo, "ssl", fmt.Sprintf("Auto-detected SSL certificate #%d (%s) for %v", matchedCert.ID, matchedCert.NiceName, domains), "")
		*certID = matchedCert.ID
		if !explicitForced && s.cfg.AutoSSLForced {
			*sslForced = true
		}
	}
}

// reconcileProxyInNPM checks if proxy host already exists, creates or updates as needed.
func (s *Syncer) reconcileProxyInNPM(ctx context.Context, inspect *docker.ContainerInspect, proxyCfg *ContainerProxyConfig, resMethod string) {
	if s.npmClient == nil {
		return
	}

	// Auto-detect SSL certificate matching domains if not explicitly configured
	s.applyAutoDetectSSL(ctx, proxyCfg.DomainNames, &proxyCfg.CertificateID, &proxyCfg.SSLForced, proxyCfg.ExplicitCertID, proxyCfg.ExplicitSSLForced)
	if proxyCfg.CertificateID != 0 && proxyCfg.CertificateID != nil {
		proxyCfg.SSLEnabled = true
	}

	// Fetch all current proxy hosts from NPM
	existingHosts, err := s.npmClient.GetProxyHosts(ctx)
	if err != nil {
		s.EmitLog(LevelError, "npm", "Failed to retrieve existing proxy hosts from NPM", err.Error())
		return
	}

	var matchedHost *npm.ProxyHost
	for i := range existingHosts {
		host := &existingHosts[i]
		if domainsOverlap(host.DomainNames, proxyCfg.DomainNames) {
			matchedHost = host
			break
		}
	}

	// Prepare desired advanced config with metadata header and host identity
	advConfig := fmt.Sprintf("%s [host_id: %s]\n", HeaderComment, s.cfg.HostID)
	if proxyCfg.AdvancedConfig != "" {
		advConfig += proxyCfg.AdvancedConfig + "\n"
	}

	var npmLocations []npm.ProxyHostLocation
	for _, loc := range proxyCfg.Locations {
		npmLocations = append(npmLocations, npm.ProxyHostLocation{
			Path:           loc.Path,
			ForwardScheme:  loc.ForwardScheme,
			ForwardHost:    loc.ForwardHost,
			ForwardPort:    loc.ForwardPort,
			AdvancedConfig: loc.AdvancedConfig,
		})
	}

	desiredReq := &npm.ProxyHostRequest{
		DomainNames:           proxyCfg.DomainNames,
		ForwardScheme:         proxyCfg.ForwardScheme,
		ForwardHost:           proxyCfg.ForwardHost,
		ForwardPort:           proxyCfg.ForwardPort,
		CertificateID:         proxyCfg.CertificateID,
		SSLForced:             proxyCfg.SSLForced,
		HSTSEnabled:           proxyCfg.HSTSEnabled,
		HSTSSubdomains:        proxyCfg.HSTSSubdomains,
		CachingEnabled:        proxyCfg.CachingEnabled,
		AllowWebsocketUpgrade: proxyCfg.AllowWebsocketUpgrade,
		BlockExploits:         proxyCfg.BlockExploits,
		HTTP2Support:          proxyCfg.HTTP2Support,
		AdvancedConfig:        advConfig,
		Locations:             npmLocations,
		AccessListID:          npm.ParseAccessListID(proxyCfg.AccessListID),
		Meta: map[string]interface{}{
			"managed_by":        ManagedByTag,
			"host_id":           s.cfg.HostID,
			"container_id":      proxyCfg.ContainerID,
			"container_name":    proxyCfg.ContainerName,
			"resolution_method": resMethod,
			"synced_at":         time.Now().Format(time.RFC3339),
		},
	}

	if matchedHost == nil {
		// Create new proxy host
		s.EmitLog(LevelInfo, "npm", fmt.Sprintf("Creating proxy host in NPM for %v -> %s:%d (host: %s)", proxyCfg.DomainNames, proxyCfg.ForwardHost, proxyCfg.ForwardPort, s.cfg.HostID), "")
		created, err := s.npmClient.CreateProxyHost(ctx, desiredReq)
		if err != nil {
			s.EmitLog(LevelError, "npm", fmt.Sprintf("Failed to create proxy host for %v", proxyCfg.DomainNames), err.Error())
			return
		}

		s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Successfully created proxy host ID #%d for %v (host: %s)", created.ID, created.DomainNames, s.cfg.HostID), fmt.Sprintf("Target: %s:%d", created.ForwardHost, created.ForwardPort))
		s.saveTrackedProxy(created.ID, inspect, proxyCfg, resMethod, "active")

	} else {
		// Check ownership in multi-host setups
		isManaged, _, hostOwner, _, _, _ := getManagedInfo(matchedHost)
		if isManaged && hostOwner != "" && hostOwner != s.cfg.HostID {
			s.EmitLog(LevelWarn, "sync", fmt.Sprintf("Domain(s) %v are managed by another host (%s). Skipping update on this node (%s) to prevent conflict.", proxyCfg.DomainNames, hostOwner, s.cfg.HostID), "")
			return
		}

		// Check if host needs updating (idempotency check)
		needsUpdate, diffReason := isUpdateRequired(matchedHost, desiredReq)
		if !needsUpdate {
			s.EmitLog(LevelInfo, "sync", fmt.Sprintf("Proxy host #%d for %v is already up to date (skipping NPM update)", matchedHost.ID, matchedHost.DomainNames), "")
			s.saveTrackedProxy(matchedHost.ID, inspect, proxyCfg, resMethod, "active")
			return
		}

		s.EmitLog(LevelInfo, "npm", fmt.Sprintf("Updating proxy host ID #%d for %v (%s)", matchedHost.ID, proxyCfg.DomainNames, diffReason), "")
		updated, err := s.npmClient.UpdateProxyHost(ctx, matchedHost.ID, desiredReq)
		if err != nil {
			s.EmitLog(LevelError, "npm", fmt.Sprintf("Failed to update proxy host #%d", matchedHost.ID), err.Error())
			return
		}

		s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Successfully updated proxy host ID #%d for %v", updated.ID, updated.DomainNames), diffReason)
		s.saveTrackedProxy(updated.ID, inspect, proxyCfg, resMethod, "active")
	}
}

// cleanupContainerProxies removes proxy hosts associated with a container that stopped or died on this host.
func (s *Syncer) cleanupContainerProxies(ctx context.Context, containerID, containerName string) {
	s.cancelPendingHealthCheck(containerID)

	if s.npmClient == nil {
		return
	}

	existingHosts, err := s.npmClient.GetProxyHosts(ctx)
	if err != nil {
		s.EmitLog(LevelError, "npm", "Failed to query proxy hosts during container cleanup", err.Error())
		return
	}

	for _, host := range existingHosts {
		if s.isManagedByThisHostAndContainer(&host, containerID, containerName) {
			s.EmitLog(LevelInfo, "npm", fmt.Sprintf("Deleting proxy host #%d for stopped container '%s' on host '%s' (domains: %v)", host.ID, containerName, s.cfg.HostID, host.DomainNames), "")
			if err := s.npmClient.DeleteProxyHost(ctx, host.ID); err != nil {
				s.EmitLog(LevelError, "npm", fmt.Sprintf("Failed to delete proxy host #%d", host.ID), err.Error())
			} else {
				s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Successfully removed proxy host #%d (%v)", host.ID, host.DomainNames), "")
				s.removeTrackedProxy(containerID, host.DomainNames)
			}
		}
	}
}

type readyEndpoint struct {
	inspect   *docker.ContainerInspect
	proxyCfg  *ContainerProxyConfig
	resMethod string
}

// reconcileDomainGroup aggregates multi-container microservices and dynamically load-balances scaled replicas.
func (s *Syncer) reconcileDomainGroup(ctx context.Context, primaryDomain string, eps []*readyEndpoint) {
	if len(eps) == 0 {
		return
	}

	if len(eps) == 1 {
		// Single container
		s.reconcileProxyInNPM(ctx, eps[0].inspect, eps[0].proxyCfg, eps[0].resMethod)
		return
	}

	// Multi-container domain: Microservices & scaled load balancing (Phase 2.1 & 2.3)
	pathEndpoints := make(map[string][]*readyEndpoint)
	for _, ep := range eps {
		p := ep.proxyCfg.Path
		if p == "" {
			p = "/"
		}
		pathEndpoints[p] = append(pathEndpoints[p], ep)
	}

	var upstreamBlocks []string
	pathTargets := make(map[string]string)
	pathPorts := make(map[string]int)

	for path, reps := range pathEndpoints {
		if len(reps) > 1 {
			// Dynamic Upstream Load Balancing for Scaled Services (Phase 2.3)
			upName := upstream.SanitizeUpstreamName(primaryDomain, path)
			var servers []upstream.Server
			for _, r := range reps {
				target := fmt.Sprintf("%s:%d", r.proxyCfg.ForwardHost, r.proxyCfg.ForwardPort)
				servers = append(servers, upstream.Server{
					Target:      target,
					MaxFails:    r.proxyCfg.Upstream.MaxFails,
					FailTimeout: r.proxyCfg.Upstream.FailTimeout,
				})
			}
			block := upstream.GenerateUpstreamBlock(upstream.Config{
				Domain:    primaryDomain,
				Path:      path,
				Algorithm: reps[0].proxyCfg.Upstream.Algorithm,
				Servers:   servers,
			})
			upstreamBlocks = append(upstreamBlocks, block)
			pathTargets[path] = upName
			pathPorts[path] = reps[0].proxyCfg.ForwardPort
			s.EmitLog(LevelInfo, "sync", fmt.Sprintf("Dynamic Upstream: Balanced %d replicas for domain '%s' path '%s' -> %s", len(reps), primaryDomain, path, upName), "")
		} else {
			pathTargets[path] = reps[0].proxyCfg.ForwardHost
			pathPorts[path] = reps[0].proxyCfg.ForwardPort
		}
	}

	// Base endpoint selection: prioritize root "/"
	var baseEP *readyEndpoint
	if rootReps, ok := pathEndpoints["/"]; ok && len(rootReps) > 0 {
		baseEP = rootReps[0]
	} else {
		baseEP = eps[0]
	}

	unifiedCfg := *baseEP.proxyCfg
	if rootTarget, ok := pathTargets["/"]; ok {
		unifiedCfg.ForwardHost = rootTarget
		unifiedCfg.ForwardPort = pathPorts["/"]
	}

	// Aggregate domain names
	domainSet := make(map[string]bool)
	var allDomains []string
	for _, ep := range eps {
		for _, d := range ep.proxyCfg.DomainNames {
			if !domainSet[d] {
				domainSet[d] = true
				allDomains = append(allDomains, d)
			}
		}
	}
	unifiedCfg.DomainNames = allDomains

	// Aggregate advanced config: upstream blocks + base advanced config
	var advParts []string
	if len(upstreamBlocks) > 0 {
		advParts = append(advParts, strings.Join(upstreamBlocks, "\n\n"))
	}
	if baseEP.proxyCfg.AdvancedConfig != "" {
		advParts = append(advParts, baseEP.proxyCfg.AdvancedConfig)
	}
	unifiedCfg.AdvancedConfig = strings.Join(advParts, "\n\n")

	// Assemble subpath locations (Phase 2.1)
	var aggregatedLocations []ContainerLocationConfig
	seenPaths := make(map[string]bool)

	for path, reps := range pathEndpoints {
		if path == "/" {
			continue
		}
		seenPaths[path] = true
		rep := reps[0]
		locAdv := rep.proxyCfg.AdvancedConfig
		if rep.proxyCfg.StripPrefix {
			cleanPfx := strings.TrimSuffix(path, "/")
			rewriteRule := fmt.Sprintf("rewrite ^%s/?(.*)$ /$1 break;", cleanPfx)
			if locAdv != "" {
				locAdv = rewriteRule + "\n\n" + locAdv
			} else {
				locAdv = rewriteRule
			}
		}
		if rep.proxyCfg.AllowWebsocketUpgrade {
			wsHeader := "proxy_set_header Upgrade $http_upgrade;\nproxy_set_header Connection \"upgrade\";"
			if locAdv != "" {
				locAdv = wsHeader + "\n\n" + locAdv
			} else {
				locAdv = wsHeader
			}
		}

		aggregatedLocations = append(aggregatedLocations, ContainerLocationConfig{
			Path:                  path,
			ForwardScheme:         rep.proxyCfg.ForwardScheme,
			ForwardHost:           pathTargets[path],
			ForwardPort:           pathPorts[path],
			StripPrefix:           rep.proxyCfg.StripPrefix,
			AllowWebsocketUpgrade: rep.proxyCfg.AllowWebsocketUpgrade,
			AdvancedConfig:        locAdv,
			Middlewares:           rep.proxyCfg.Middlewares,
		})
	}

	// Add any explicitly defined custom locations from labels
	for _, ep := range eps {
		for _, loc := range ep.proxyCfg.Locations {
			if !seenPaths[loc.Path] {
				seenPaths[loc.Path] = true
				aggregatedLocations = append(aggregatedLocations, loc)
			}
		}
	}
	unifiedCfg.Locations = aggregatedLocations

	// Aggregate container identifiers
	var cIDs []string
	var cNames []string
	for _, ep := range eps {
		cIDs = append(cIDs, ep.proxyCfg.ContainerID)
		cNames = append(cNames, ep.proxyCfg.ContainerName)
	}
	unifiedCfg.ContainerID = strings.Join(cIDs, ",")
	unifiedCfg.ContainerName = strings.Join(cNames, ",")

	s.reconcileProxyInNPM(ctx, baseEP.inspect, &unifiedCfg, "multi-container microservice ingress")
}

// reconcileStreams synchronizes Layer 4 TCP/UDP streams from Docker labels, IaC manifests, Proxmox LXC, and LXD/Incus with NPM.
func (s *Syncer) reconcileStreams(ctx context.Context, dockerStreams []ContainerStreamConfig, iacStreams []iac.StaticStream, extraStreams ...any) int {
	if s.npmClient == nil {
		return 0
	}

	desiredStreams := make(map[int]*npm.StreamRequest)
	streamMeta := make(map[int]map[string]interface{})

	// 1. Process Docker streams
	for _, ds := range dockerStreams {
		fHost := ds.ForwardingHost
		if fHost == "" {
			if s.cfg.HostIP != "" && s.cfg.UseHostPort {
				fHost = s.cfg.HostIP
			} else {
				fHost = ds.ContainerName
			}
		}
		fPort := ds.ForwardingPort
		if fPort <= 0 {
			fPort = ds.IncomingPort
		}

		req := &npm.StreamRequest{
			IncomingPort:   ds.IncomingPort,
			ForwardingHost: fHost,
			ForwardingPort: fPort,
			TCPForwarding:  ds.TCPForwarding,
			UDPForwarding:  ds.UDPForwarding,
			Meta: map[string]interface{}{
				"managed_by":     ManagedByTag,
				"host_id":        s.cfg.HostID,
				"container_id":   ds.ContainerID,
				"container_name": ds.ContainerName,
				"source":         "docker",
				"synced_at":      time.Now().Format(time.RFC3339),
			},
		}
		desiredStreams[ds.IncomingPort] = req
		streamMeta[ds.IncomingPort] = map[string]interface{}{
			"container_id":   ds.ContainerID,
			"container_name": ds.ContainerName,
			"source":         "docker",
			"resolution":     fHost,
		}
	}

	// 2. Process IaC streams
	for _, is := range iacStreams {
		if _, exists := desiredStreams[is.IncomingPort]; !exists {
			tcp := true
			udp := false
			if is.TCPForwarding != nil {
				tcp = *is.TCPForwarding
			}
			if is.UDPForwarding != nil {
				udp = *is.UDPForwarding
			}
			fPort := is.ForwardingPort
			if fPort <= 0 {
				fPort = is.IncomingPort
			}

			req := &npm.StreamRequest{
				IncomingPort:   is.IncomingPort,
				ForwardingHost: is.ForwardingHost,
				ForwardingPort: fPort,
				TCPForwarding:  tcp,
				UDPForwarding:  udp,
				Meta: map[string]interface{}{
					"managed_by":  ManagedByTag,
					"host_id":     s.cfg.HostID,
					"source":      "iac",
					"source_file": is.SourceFile,
					"synced_at":   time.Now().Format(time.RFC3339),
				},
			}
			desiredStreams[is.IncomingPort] = req
			streamMeta[is.IncomingPort] = map[string]interface{}{
				"source":      "iac",
				"source_file": is.SourceFile,
				"resolution":  is.ForwardingHost,
			}
		}
	}

	// 3. Process Proxmox VE LXC and LXD/Incus streams
	var pveStreams []pve.PVEStream
	var lxdStreams []lxd.LXDStream
	for _, extra := range extraStreams {
		switch v := extra.(type) {
		case []pve.PVEStream:
			pveStreams = append(pveStreams, v...)
		case []lxd.LXDStream:
			lxdStreams = append(lxdStreams, v...)
		}
	}

	for _, ps := range pveStreams {
		if _, exists := desiredStreams[ps.IncomingPort]; !exists {
			fPort := ps.ForwardingPort
			if fPort <= 0 {
				fPort = ps.IncomingPort
			}
			req := &npm.StreamRequest{
				IncomingPort:   ps.IncomingPort,
				ForwardingHost: ps.ForwardingHost,
				ForwardingPort: fPort,
				TCPForwarding:  ps.TCPForwarding,
				UDPForwarding:  ps.UDPForwarding,
				Meta: map[string]interface{}{
					"managed_by":     ManagedByTag,
					"host_id":        s.cfg.HostID,
					"source":         "pve",
					"vmid":           ps.VMID,
					"container_name": ps.ContainerName,
					"node":           ps.Node,
					"synced_at":      time.Now().Format(time.RFC3339),
				},
			}
			desiredStreams[ps.IncomingPort] = req
			streamMeta[ps.IncomingPort] = map[string]interface{}{
				"source":         "pve",
				"container_id":   fmt.Sprintf("pve:%d", ps.VMID),
				"container_name": fmt.Sprintf("%s:%s", ps.Node, ps.ContainerName),
				"resolution":     ps.ForwardingHost,
			}
		}
	}

	for _, ls := range lxdStreams {
		if _, exists := desiredStreams[ls.IncomingPort]; !exists {
			fPort := ls.ForwardingPort
			if fPort <= 0 {
				fPort = ls.IncomingPort
			}
			req := &npm.StreamRequest{
				IncomingPort:   ls.IncomingPort,
				ForwardingHost: ls.ForwardingHost,
				ForwardingPort: fPort,
				TCPForwarding:  ls.TCPForwarding,
				UDPForwarding:  ls.UDPForwarding,
				Meta: map[string]interface{}{
					"managed_by":     ManagedByTag,
					"host_id":        s.cfg.HostID,
					"source":         "lxd",
					"container_name": ls.Name,
					"synced_at":      time.Now().Format(time.RFC3339),
				},
			}
			desiredStreams[ls.IncomingPort] = req
			streamMeta[ls.IncomingPort] = map[string]interface{}{
				"source":         "lxd",
				"container_id":   fmt.Sprintf("lxd:%s", ls.Name),
				"container_name": ls.Name,
				"resolution":     ls.ForwardingHost,
			}
		}
	}

	existingStreams, err := s.npmClient.GetStreams(ctx)
	if err != nil {
		s.EmitLog(LevelError, "npm", "Failed to retrieve existing streams from NPM", err.Error())
		return 0
	}

	syncedCount := 0
	existingByPort := make(map[int]npm.Stream)
	for _, es := range existingStreams {
		existingByPort[es.IncomingPort] = es
	}

	for inPort, desiredReq := range desiredStreams {
		matched, exists := existingByPort[inPort]
		metaInfo := streamMeta[inPort]
		cID, _ := metaInfo["container_id"].(string)
		cName, _ := metaInfo["container_name"].(string)
		source, _ := metaInfo["source"].(string)
		srcFile, _ := metaInfo["source_file"].(string)
		resMethod, _ := metaInfo["resolution"].(string)

		if !exists {
			s.EmitLog(LevelInfo, "npm", fmt.Sprintf("Creating stream in NPM on port %d -> %s:%d (tcp:%v, udp:%v)", desiredReq.IncomingPort, desiredReq.ForwardingHost, desiredReq.ForwardingPort, desiredReq.TCPForwarding, desiredReq.UDPForwarding), "")
			created, err := s.npmClient.CreateStream(ctx, desiredReq)
			if err != nil {
				s.EmitLog(LevelError, "npm", fmt.Sprintf("Failed to create stream on port %d", desiredReq.IncomingPort), err.Error())
				continue
			}
			s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Successfully created stream ID #%d on port %d", created.ID, created.IncomingPort), "")
			s.saveTrackedStream(created.ID, desiredReq, cID, cName, source, srcFile, resMethod, "active")
			syncedCount++
		} else {
			// Check multi-host ownership
			isManaged, _, hostOwner, _ := getStreamManagedInfo(&matched)
			if isManaged && hostOwner != "" && hostOwner != s.cfg.HostID {
				s.EmitLog(LevelWarn, "sync", fmt.Sprintf("Stream on port %d is managed by another host (%s). Skipping update on this node (%s).", matched.IncomingPort, hostOwner, s.cfg.HostID), "")
				continue
			}

			needsUpdate, diffReason := isStreamUpdateRequired(&matched, desiredReq)
			if !needsUpdate {
				s.saveTrackedStream(matched.ID, desiredReq, cID, cName, source, srcFile, resMethod, "active")
				syncedCount++
				continue
			}

			s.EmitLog(LevelInfo, "npm", fmt.Sprintf("Updating stream ID #%d on port %d (%s)", matched.ID, matched.IncomingPort, diffReason), "")
			updated, err := s.npmClient.UpdateStream(ctx, matched.ID, desiredReq)
			if err != nil {
				s.EmitLog(LevelError, "npm", fmt.Sprintf("Failed to update stream #%d", matched.ID), err.Error())
				continue
			}
			s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Successfully updated stream ID #%d", updated.ID), diffReason)
			s.saveTrackedStream(updated.ID, desiredReq, cID, cName, source, srcFile, resMethod, "active")
			syncedCount++
		}
	}

	// Orphan stream cleanup
	for _, es := range existingStreams {
		isManaged, _, hostOwner, _ := getStreamManagedInfo(&es)
		if isManaged && (hostOwner == "" || hostOwner == s.cfg.HostID) {
			if _, stillDesired := desiredStreams[es.IncomingPort]; !stillDesired {
				s.EmitLog(LevelWarn, "sync", fmt.Sprintf("Found orphaned stream #%d on port %d - pruning from NPM", es.ID, es.IncomingPort), "")
				if err := s.npmClient.DeleteStream(ctx, es.ID); err == nil {
					s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Pruned orphan stream #%d on port %d", es.ID, es.IncomingPort), "")
					s.removeTrackedStream(es.IncomingPort)
				}
			}
		}
	}

	return syncedCount
}

// performFullSync scans running containers and static IaC manifests, reconciling all with NPM.
func (s *Syncer) performFullSync(ctx context.Context, triggerSource string) {
	syncStart := time.Now()
	s.EmitLog(LevelInfo, "sync", fmt.Sprintf("Running full discovery scan on host '%s' (%s)", s.cfg.HostID, triggerSource), "")

	// 1. Docker Containers Discovery
	runningContainerIDs := make(map[string]bool)
	runningContainerNames := make(map[string]bool)
	var allDockerStreams []ContainerStreamConfig
	domainGroups := make(map[string][]*readyEndpoint)
	activeDockerDomains := make(map[string]bool)
	syncedDockerCount := 0

	if s.dockerClient != nil {
		containers, err := s.dockerClient.ListContainers(ctx, false)
		if err == nil {
			for _, c := range containers {
				runningContainerIDs[c.ID] = true
				for _, name := range c.Names {
					runningContainerNames[strings.TrimPrefix(name, "/")] = true
				}

				inspect, err := s.dockerClient.InspectContainer(ctx, c.ID)
				if err != nil {
					continue
				}

				// Layer 4 TCP/UDP Streams Discovery (Phase 2.2)
				if streams, hasStreams, _ := ParseContainerStreams(inspect, s.cfg); hasStreams {
					allDockerStreams = append(allDockerStreams, streams...)
				}

				proxyCfg, shouldProxy, err := ParseContainerLabels(inspect, s.cfg)
				if err != nil || !shouldProxy || proxyCfg == nil {
					continue
				}

				targetHost, resMethod, err := ResolveForwardHost(inspect, proxyCfg, s.cfg)
				if err != nil {
					s.EmitLog(LevelWarn, "sync", fmt.Sprintf("Could not resolve host for %s: %s", inspect.Name, err), "")
					continue
				}
				proxyCfg.ForwardHost = targetHost

				// Zero-502 HealthCheck Check (Phase 1)
				if s.shouldHoldForHealthCheck(inspect, proxyCfg) {
					s.queuePendingHealthCheck(ctx, inspect, proxyCfg, resMethod)
					continue
				}

				// Clean up any pending timer if now healthy
				s.cancelPendingHealthCheck(c.ID)

				primaryDomain := strings.ToLower(proxyCfg.DomainNames[0])
				domainGroups[primaryDomain] = append(domainGroups[primaryDomain], &readyEndpoint{
					inspect:   inspect,
					proxyCfg:  proxyCfg,
					resMethod: resMethod,
				})
			}

			// Reconcile aggregated domain groups (Phase 2.1 Microservices & 2.3 Load Balancing)
			for primaryDomain, eps := range domainGroups {
				activeDockerDomains[primaryDomain] = true
				s.reconcileDomainGroup(ctx, primaryDomain, eps)
				syncedDockerCount++
			}
		} else {
			s.EmitLog(LevelWarn, "docker", "Could not list Docker containers (Docker socket may be inactive)", err.Error())
		}
	}

	// 2. Infrastructure-as-Code (IaC) Manifests Sync
	activeIaCDomains := make(map[string]bool)
	syncedIaCCount := 0
	if s.cfg.RoutesFile != "" || s.cfg.RoutesDir != "" {
		syncedIaCCount = s.syncIaCRoutes(ctx, activeIaCDomains)
	}

	// 3. Proxmox VE (PVE) LXC Discovery (Phase 3.1)
	activePVEDomains := make(map[string]bool)
	var allPVEStreams []pve.PVEStream
	var allPVEContainers []pve.LXCContainerSummary
	syncedPVECount := 0

	s.mu.RLock()
	clientsToRun := make([]*pve.Client, 0, len(s.pveClients))
	for _, cl := range s.pveClients {
		if cl != nil {
			clientsToRun = append(clientsToRun, cl)
		}
	}
	if len(clientsToRun) == 0 && s.pveClient != nil {
		clientsToRun = append(clientsToRun, s.pveClient)
	}
	s.mu.RUnlock()

	for _, client := range clientsToRun {
		pveRoutes, pveStreams, pveContainers, err := client.DiscoverRoutes(ctx, s.cfg.DefaultForwardScheme, s.cfg.DefaultSSLEnabled, s.cfg.DefaultWebsocket, s.cfg.DefaultBlockExploits)
		if err == nil {
			allPVEContainers = append(allPVEContainers, pveContainers...)
			allPVEStreams = append(allPVEStreams, pveStreams...)
			for _, r := range pveRoutes {
				if len(r.DomainNames) > 0 {
					activePVEDomains[strings.ToLower(r.DomainNames[0])] = true
				}
				s.reconcilePVERoute(ctx, r)
				syncedPVECount++
			}
		} else {
			s.EmitLog(LevelWarn, "pve", fmt.Sprintf("Proxmox VE discovery error (%s)", client.BaseURL()), err.Error())
		}
	}

	if len(clientsToRun) > 0 {
		s.mu.Lock()
		s.pveContainers = allPVEContainers
		s.mu.Unlock()
	}

	// 4. Canonical LXD / Incus Discovery (Phase 3.2)
	activeLXDDomains := make(map[string]bool)
	var allLXDStreams []lxd.LXDStream
	syncedLXDCount := 0
	if s.lxdClient != nil {
		lxdRoutes, lxdStreams, lxdInstances, err := s.lxdClient.DiscoverRoutes(ctx, s.cfg.DefaultForwardScheme, s.cfg.DefaultSSLEnabled, s.cfg.DefaultWebsocket, s.cfg.DefaultBlockExploits)
		if err == nil {
			s.mu.Lock()
			s.lxdInstances = lxdInstances
			s.mu.Unlock()

			allLXDStreams = lxdStreams
			for _, r := range lxdRoutes {
				if len(r.DomainNames) > 0 {
					activeLXDDomains[strings.ToLower(r.DomainNames[0])] = true
				}
				s.reconcileLXDRoute(ctx, r)
				syncedLXDCount++
			}
		} else {
			s.EmitLog(LevelWarn, "lxd", "Canonical LXD / Incus discovery error", err.Error())
		}
	}

	// 5. Layer 4 Streams Sync (Phase 2.2, 3.1 & 3.2)
	var allIaCStreams []iac.StaticStream
	if s.cfg.RoutesFile != "" {
		if ss, err := iac.LoadStreams(s.cfg.RoutesFile); err == nil {
			allIaCStreams = append(allIaCStreams, ss...)
		}
	}
	if s.cfg.RoutesDir != "" {
		if ss, err := iac.LoadStreams(s.cfg.RoutesDir); err == nil {
			allIaCStreams = append(allIaCStreams, ss...)
		}
	}
	s.reconcileStreams(ctx, allDockerStreams, allIaCStreams, allPVEStreams, allLXDStreams)

	// 6. Orphan Proxy Host Cleanup & Manual NPM Proxy Sync
	s.syncManualProxiesFromNPM(ctx)
	existingHosts, err := s.npmClient.GetProxyHosts(ctx)
	if err == nil {
		for _, host := range existingHosts {
			isManaged, source, hostOwner, cID, cName, _ := getManagedInfo(&host)
			if isManaged {
				// Safety check for multi-host clusters: NEVER delete proxies belonging to another host!
				if hostOwner != "" && hostOwner != s.cfg.HostID {
					continue
				}

				if source == "iac" {
					// Only prune IaC proxy if IaC provider is configured and domain is missing from manifests
					if (s.cfg.RoutesFile != "" || s.cfg.RoutesDir != "") && len(host.DomainNames) > 0 {
						primaryDomain := strings.ToLower(host.DomainNames[0])
						if !activeIaCDomains[primaryDomain] {
							s.EmitLog(LevelWarn, "sync", fmt.Sprintf("Found orphaned IaC proxy host #%d (domains: %v) removed from manifests - pruning", host.ID, host.DomainNames), "")
							if delErr := s.npmClient.DeleteProxyHost(ctx, host.ID); delErr == nil {
								s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Pruned orphan IaC proxy host #%d", host.ID), "")
								s.removeTrackedProxy(cID, host.DomainNames)
							}
						}
					}
				} else if source == "pve" {
					if (s.pveClient != nil || len(s.pveClients) > 0) && len(host.DomainNames) > 0 {
						primaryDomain := strings.ToLower(host.DomainNames[0])
						if !activePVEDomains[primaryDomain] {
							s.EmitLog(LevelWarn, "sync", fmt.Sprintf("Found orphaned Proxmox LXC proxy host #%d (domains: %v) - pruning", host.ID, host.DomainNames), "")
							if delErr := s.npmClient.DeleteProxyHost(ctx, host.ID); delErr == nil {
								s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Pruned orphan PVE proxy host #%d", host.ID), "")
								s.removeTrackedProxy(cID, host.DomainNames)
							}
						}
					}
				} else if source == "lxd" {
					if s.lxdClient != nil && len(host.DomainNames) > 0 {
						primaryDomain := strings.ToLower(host.DomainNames[0])
						if !activeLXDDomains[primaryDomain] {
							s.EmitLog(LevelWarn, "sync", fmt.Sprintf("Found orphaned LXD/Incus proxy host #%d (domains: %v) - pruning", host.ID, host.DomainNames), "")
							if delErr := s.npmClient.DeleteProxyHost(ctx, host.ID); delErr == nil {
								s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Pruned orphan LXD proxy host #%d", host.ID), "")
								s.removeTrackedProxy(cID, host.DomainNames)
							}
						}
					}
				} else {
					// Docker proxy host orphan cleanup
					primaryDomain := ""
					if len(host.DomainNames) > 0 {
						primaryDomain = strings.ToLower(host.DomainNames[0])
					}

					if primaryDomain != "" && !activeDockerDomains[primaryDomain] {
						s.EmitLog(LevelWarn, "sync", fmt.Sprintf("Found orphaned Docker proxy host #%d for container '%s' on host '%s' (domains: %v) - pruning", host.ID, cName, s.cfg.HostID, host.DomainNames), "")
						if delErr := s.npmClient.DeleteProxyHost(ctx, host.ID); delErr == nil {
							s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Pruned orphan Docker proxy host #%d", host.ID), "")
							s.removeTrackedProxy(cID, host.DomainNames)
						}
					}
				}
			}
		}
	}

	syncDuration := time.Since(syncStart)
	metrics.ObserveSyncDuration(syncDuration)
	metrics.SetActiveProxies(s.cfg.HostID, "docker", syncedDockerCount)
	metrics.SetActiveProxies(s.cfg.HostID, "iac", syncedIaCCount)
	metrics.SetActiveProxies(s.cfg.HostID, "pve", syncedPVECount)
	metrics.SetActiveProxies(s.cfg.HostID, "lxd", syncedLXDCount)
	metrics.SetActiveStreams(s.cfg.HostID, "docker", len(allDockerStreams))
	metrics.SetActiveStreams(s.cfg.HostID, "iac", len(allIaCStreams))
	metrics.SetActiveStreams(s.cfg.HostID, "pve", len(allPVEStreams))
	metrics.SetActiveStreams(s.cfg.HostID, "lxd", len(allLXDStreams))

	s.mu.Lock()
	s.lastFullSync = time.Now()
	s.mu.Unlock()

	s.EmitLog(LevelSuccess, "sync", fmt.Sprintf("Full sync completed in %s. Processed %d Docker, %d IaC, %d PVE, %d LXD proxies, and %d streams",
		syncDuration.Round(time.Millisecond),
		syncedDockerCount,
		syncedIaCCount,
		syncedPVECount,
		syncedLXDCount,
		len(allDockerStreams)+len(allIaCStreams)+len(allPVEStreams)+len(allLXDStreams)), "")

	s.triggerWorkerPush()
}

// syncIaCRoutes loads and applies declarative routes from static YAML/JSON manifests.
func (s *Syncer) syncIaCRoutes(ctx context.Context, activeDomains map[string]bool) int {
	var allHosts []iac.StaticProxyHost

	if s.cfg.RoutesFile != "" {
		hosts, err := iac.LoadRoutes(s.cfg.RoutesFile)
		if err != nil {
			s.EmitLog(LevelError, "iac", fmt.Sprintf("Failed loading routes file '%s'", s.cfg.RoutesFile), err.Error())
		} else {
			allHosts = append(allHosts, hosts...)
		}
	}

	if s.cfg.RoutesDir != "" {
		hosts, err := iac.LoadRoutes(s.cfg.RoutesDir)
		if err != nil {
			s.EmitLog(LevelError, "iac", fmt.Sprintf("Failed loading routes directory '%s'", s.cfg.RoutesDir), err.Error())
		} else {
			allHosts = append(allHosts, hosts...)
		}
	}

	syncedCount := 0
	for _, sh := range allHosts {
		if len(sh.DomainNames) > 0 {
			activeDomains[strings.ToLower(sh.DomainNames[0])] = true
		}
		s.reconcileIaCHost(ctx, sh)
		syncedCount++
	}

	return syncedCount
}

// reconcileIaCHost creates or updates a proxy host declared in static IaC manifests.
func (s *Syncer) reconcileIaCHost(ctx context.Context, sh iac.StaticProxyHost) {
	if s.npmClient == nil {
		return
	}

	existingHosts, err := s.npmClient.GetProxyHosts(ctx)
	if err != nil {
		s.EmitLog(LevelError, "npm", "Failed to retrieve existing proxy hosts for IaC reconcile", err.Error())
		return
	}

	var matchedHost *npm.ProxyHost
	for i := range existingHosts {
		host := &existingHosts[i]
		if domainsOverlap(host.DomainNames, sh.DomainNames) {
			matchedHost = host
			break
		}
	}

	advConfig := fmt.Sprintf("%s [source: iac] [host_id: %s]\n", HeaderComment, s.cfg.HostID)
	if sh.AdvancedConfig != "" {
		advConfig += sh.AdvancedConfig + "\n"
	}

	var npmLocations []npm.ProxyHostLocation
	for _, loc := range sh.Locations {
		npmLocations = append(npmLocations, npm.ProxyHostLocation{
			Path:           loc.Path,
			ForwardScheme:  loc.ForwardScheme,
			ForwardHost:    loc.ForwardHost,
			ForwardPort:    loc.ForwardPort,
			AdvancedConfig: loc.AdvancedConfig,
		})
	}

	// Auto-detect SSL certificate if not explicitly set
	s.applyAutoDetectSSL(ctx, sh.DomainNames, &sh.CertificateID, &sh.SSLForced, sh.CertificateID != nil && sh.CertificateID != 0 && sh.CertificateID != "", sh.SSLForced)
	if sh.CertificateID != 0 && sh.CertificateID != nil {
		sh.SSLEnabled = true
	}

	desiredReq := &npm.ProxyHostRequest{
		DomainNames:           sh.DomainNames,
		ForwardScheme:         sh.ForwardScheme,
		ForwardHost:           sh.ForwardHost,
		ForwardPort:           sh.ForwardPort,
		CertificateID:         sh.CertificateID,
		SSLForced:             sh.SSLForced,
		HSTSEnabled:           sh.HSTSEnabled,
		HSTSSubdomains:        sh.HSTSSubdomains,
		CachingEnabled:        sh.CachingEnabled,
		AllowWebsocketUpgrade: sh.AllowWebsocketUpgrade,
		BlockExploits:         sh.BlockExploits,
		HTTP2Support:          sh.HTTP2Support,
		AdvancedConfig:        advConfig,
		Locations:             npmLocations,
		AccessListID:          npm.ParseAccessListID(sh.AccessListID),
		Meta: map[string]interface{}{
			"managed_by":  ManagedByTag,
			"source":      "iac",
			"host_id":     s.cfg.HostID,
			"source_file": sh.SourceFile,
			"synced_at":   time.Now().Format(time.RFC3339),
		},
	}

	if matchedHost == nil {
		s.EmitLog(LevelInfo, "npm", fmt.Sprintf("Creating IaC proxy host for %v -> %s:%d (file: %s)", sh.DomainNames, sh.ForwardHost, sh.ForwardPort, sh.SourceFile), "")
		created, err := s.npmClient.CreateProxyHost(ctx, desiredReq)
		if err != nil {
			s.EmitLog(LevelError, "npm", fmt.Sprintf("Failed to create IaC proxy host for %v", sh.DomainNames), err.Error())
			return
		}

		s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Successfully created IaC proxy host ID #%d for %v", created.ID, created.DomainNames), fmt.Sprintf("Target: %s:%d", created.ForwardHost, created.ForwardPort))
		s.saveTrackedIaCProxy(created.ID, sh, "active")
	} else {
		isManaged, _, hostOwner, _, _, _ := getManagedInfo(matchedHost)
		if isManaged && hostOwner != "" && hostOwner != s.cfg.HostID {
			s.EmitLog(LevelWarn, "sync", fmt.Sprintf("IaC domain(s) %v are managed by another host (%s). Skipping update.", sh.DomainNames, hostOwner), "")
			return
		}

		needsUpdate, diffReason := isUpdateRequired(matchedHost, desiredReq)
		if !needsUpdate {
			s.saveTrackedIaCProxy(matchedHost.ID, sh, "active")
			return
		}

		s.EmitLog(LevelInfo, "npm", fmt.Sprintf("Updating IaC proxy host ID #%d for %v (%s)", matchedHost.ID, sh.DomainNames, diffReason), "")
		updated, err := s.npmClient.UpdateProxyHost(ctx, matchedHost.ID, desiredReq)
		if err != nil {
			s.EmitLog(LevelError, "npm", fmt.Sprintf("Failed to update IaC proxy host #%d", matchedHost.ID), err.Error())
			return
		}

		s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Successfully updated IaC proxy host ID #%d for %v", updated.ID, updated.DomainNames), diffReason)
		s.saveTrackedIaCProxy(updated.ID, sh, "active")
	}
}

// saveTrackedIaCProxy records a static declarative proxy host into tracking state.
func (s *Syncer) saveTrackedIaCProxy(npmID int, host iac.StaticProxyHost, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var npmLocs []npm.ProxyHostLocation
	for _, l := range host.Locations {
		npmLocs = append(npmLocs, npm.ProxyHostLocation{
			Path:           l.Path,
			ForwardScheme:  l.ForwardScheme,
			ForwardHost:    l.ForwardHost,
			ForwardPort:    l.ForwardPort,
			AdvancedConfig: l.AdvancedConfig,
		})
	}

	key := strings.ToLower(host.DomainNames[0])
	s.trackedProxies[key] = &ManagedProxy{
		NPMHostID:        npmID,
		HostID:           s.cfg.HostID,
		ContainerID:      "iac-manifest",
		ContainerName:    "iac:" + host.SourceFile,
		Image:            "manifest:" + host.SourceFile,
		DomainNames:      host.DomainNames,
		ForwardScheme:    host.ForwardScheme,
		ForwardHost:      host.ForwardHost,
		ForwardPort:      host.ForwardPort,
		ResolutionMethod: "iac-manifest (" + host.SourceFile + ")",
		SSLEnabled:       host.SSLEnabled,
		SSLForced:        host.SSLForced,
		CertificateID:    host.CertificateID,
		Websocket:        host.AllowWebsocketUpgrade,
		BlockExploits:    host.BlockExploits,
		Locations:        npmLocs,
		Source:           "iac",
		SourceFile:       host.SourceFile,
		Status:           status,
		LastSynced:       time.Now(),
	}
}

// reconcilePVERoute creates or updates a proxy host discovered from Proxmox VE LXC.
func (s *Syncer) reconcilePVERoute(ctx context.Context, route pve.PVERoute) {
	if s.npmClient == nil {
		return
	}

	existingHosts, err := s.npmClient.GetProxyHosts(ctx)
	if err != nil {
		s.EmitLog(LevelError, "npm", "Failed to retrieve existing proxy hosts for PVE reconcile", err.Error())
		return
	}

	var matchedHost *npm.ProxyHost
	for i := range existingHosts {
		host := &existingHosts[i]
		if domainsOverlap(host.DomainNames, route.DomainNames) {
			matchedHost = host
			break
		}
	}

	advConfig := fmt.Sprintf("%s [source: pve] [vmid: %d] [node: %s] [host_id: %s]\n", HeaderComment, route.VMID, route.Node, s.cfg.HostID)
	if route.AdvancedConfig != "" {
		advConfig += route.AdvancedConfig + "\n"
	}

	var npmLocations []npm.ProxyHostLocation
	for _, loc := range route.Locations {
		npmLocations = append(npmLocations, npm.ProxyHostLocation{
			Path:           loc.Path,
			ForwardScheme:  loc.ForwardScheme,
			ForwardHost:    loc.ForwardHost,
			ForwardPort:    loc.ForwardPort,
			AdvancedConfig: loc.AdvancedConfig,
		})
	}

	// Auto-detect SSL certificate if not explicitly set
	s.applyAutoDetectSSL(ctx, route.DomainNames, &route.CertificateID, &route.SSLForced, route.CertificateID != nil && route.CertificateID != 0 && route.CertificateID != "", route.SSLForced)
	if route.CertificateID != 0 && route.CertificateID != nil {
		route.SSLEnabled = true
	}

	desiredReq := &npm.ProxyHostRequest{
		DomainNames:           route.DomainNames,
		ForwardScheme:         route.ForwardScheme,
		ForwardHost:           route.ForwardHost,
		ForwardPort:           route.ForwardPort,
		CertificateID:         route.CertificateID,
		SSLForced:             route.SSLForced,
		HSTSEnabled:           route.HSTSEnabled,
		HSTSSubdomains:        route.HSTSSubdomains,
		CachingEnabled:        route.CachingEnabled,
		AllowWebsocketUpgrade: route.AllowWebsocketUpgrade,
		BlockExploits:         route.BlockExploits,
		HTTP2Support:          route.HTTP2Support,
		AdvancedConfig:        advConfig,
		Locations:             npmLocations,
		AccessListID:          npm.ParseAccessListID(route.AccessListID),
		Meta: map[string]interface{}{
			"managed_by":        ManagedByTag,
			"host_id":           s.cfg.HostID,
			"source":            "pve",
			"vmid":              route.VMID,
			"node":              route.Node,
			"container_name":    route.ContainerName,
			"interface":         route.Interface,
			"resolution_method": fmt.Sprintf("pve-api (%s -> %s)", route.Interface, route.ForwardHost),
			"synced_at":         time.Now().Format(time.RFC3339),
		},
	}

	if matchedHost == nil {
		s.EmitLog(LevelInfo, "npm", fmt.Sprintf("Creating Proxmox LXC proxy host for %v -> %s:%d (LXC %d on %s)", route.DomainNames, route.ForwardHost, route.ForwardPort, route.VMID, route.Node), "")
		created, err := s.npmClient.CreateProxyHost(ctx, desiredReq)
		if err != nil {
			s.EmitLog(LevelError, "npm", fmt.Sprintf("Failed to create PVE proxy host for %v", route.DomainNames), err.Error())
			return
		}
		s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Successfully created PVE proxy host ID #%d for %v (LXC %d)", created.ID, created.DomainNames, route.VMID), fmt.Sprintf("Target: %s:%d", created.ForwardHost, created.ForwardPort))
		s.saveTrackedPVEProxy(created.ID, route, "active")
	} else {
		isManaged, _, hostOwner, _, _, _ := getManagedInfo(matchedHost)
		if isManaged && hostOwner != "" && hostOwner != s.cfg.HostID {
			s.EmitLog(LevelWarn, "sync", fmt.Sprintf("Domain(s) %v are managed by another host (%s). Skipping PVE update.", route.DomainNames, hostOwner), "")
			return
		}

		needsUpdate, diffReason := isUpdateRequired(matchedHost, desiredReq)
		if !needsUpdate {
			s.saveTrackedPVEProxy(matchedHost.ID, route, "active")
			return
		}

		s.EmitLog(LevelInfo, "npm", fmt.Sprintf("Updating PVE proxy host ID #%d for %v (%s)", matchedHost.ID, route.DomainNames, diffReason), "")
		updated, err := s.npmClient.UpdateProxyHost(ctx, matchedHost.ID, desiredReq)
		if err != nil {
			s.EmitLog(LevelError, "npm", fmt.Sprintf("Failed to update PVE proxy host #%d", matchedHost.ID), err.Error())
			return
		}
		s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Successfully updated PVE proxy host ID #%d for %v", updated.ID, updated.DomainNames), diffReason)
		s.saveTrackedPVEProxy(updated.ID, route, "active")
	}
}

// saveTrackedPVEProxy records a Proxmox LXC proxy host into tracking state.
func (s *Syncer) saveTrackedPVEProxy(npmID int, route pve.PVERoute, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var npmLocs []npm.ProxyHostLocation
	for _, l := range route.Locations {
		npmLocs = append(npmLocs, npm.ProxyHostLocation{
			Path:           l.Path,
			ForwardScheme:  l.ForwardScheme,
			ForwardHost:    l.ForwardHost,
			ForwardPort:    l.ForwardPort,
			AdvancedConfig: l.AdvancedConfig,
		})
	}

	key := strings.ToLower(route.DomainNames[0])
	s.trackedProxies[key] = &ManagedProxy{
		NPMHostID:        npmID,
		HostID:           s.cfg.HostID,
		ContainerID:      fmt.Sprintf("pve:%d", route.VMID),
		ContainerName:    fmt.Sprintf("%s:%s", route.Node, route.ContainerName),
		Image:            "lxc",
		DomainNames:      route.DomainNames,
		ForwardScheme:    route.ForwardScheme,
		ForwardHost:      route.ForwardHost,
		ForwardPort:      route.ForwardPort,
		ResolutionMethod: fmt.Sprintf("pve-api (%s -> %s)", route.Interface, route.ForwardHost),
		SSLEnabled:       route.SSLEnabled,
		SSLForced:        route.SSLForced,
		CertificateID:    route.CertificateID,
		Websocket:        route.AllowWebsocketUpgrade,
		BlockExploits:    route.BlockExploits,
		Locations:        npmLocs,
		Source:           "pve",
		Status:           status,
		LastSynced:       time.Now(),
	}
}

// reconcileLXDRoute creates or updates a proxy host discovered from Canonical LXD / Incus.
func (s *Syncer) reconcileLXDRoute(ctx context.Context, route lxd.LXDRoute) {
	if s.npmClient == nil {
		return
	}

	existingHosts, err := s.npmClient.GetProxyHosts(ctx)
	if err != nil {
		s.EmitLog(LevelError, "npm", "Failed to retrieve existing proxy hosts for LXD reconcile", err.Error())
		return
	}

	var matchedHost *npm.ProxyHost
	for i := range existingHosts {
		host := &existingHosts[i]
		if domainsOverlap(host.DomainNames, route.DomainNames) {
			matchedHost = host
			break
		}
	}

	advConfig := fmt.Sprintf("%s [source: lxd] [instance: %s] [host_id: %s]\n", HeaderComment, route.Name, s.cfg.HostID)
	if route.AdvancedConfig != "" {
		advConfig += route.AdvancedConfig + "\n"
	}

	var npmLocations []npm.ProxyHostLocation
	for _, loc := range route.Locations {
		npmLocations = append(npmLocations, npm.ProxyHostLocation{
			Path:           loc.Path,
			ForwardScheme:  loc.ForwardScheme,
			ForwardHost:    loc.ForwardHost,
			ForwardPort:    loc.ForwardPort,
			AdvancedConfig: loc.AdvancedConfig,
		})
	}

	// Auto-detect SSL certificate if not explicitly set
	s.applyAutoDetectSSL(ctx, route.DomainNames, &route.CertificateID, &route.SSLForced, route.CertificateID != nil && route.CertificateID != 0 && route.CertificateID != "", route.SSLForced)
	if route.CertificateID != 0 && route.CertificateID != nil {
		route.SSLEnabled = true
	}

	desiredReq := &npm.ProxyHostRequest{
		DomainNames:           route.DomainNames,
		ForwardScheme:         route.ForwardScheme,
		ForwardHost:           route.ForwardHost,
		ForwardPort:           route.ForwardPort,
		CertificateID:         route.CertificateID,
		SSLForced:             route.SSLForced,
		HSTSEnabled:           route.HSTSEnabled,
		HSTSSubdomains:        route.HSTSSubdomains,
		CachingEnabled:        route.CachingEnabled,
		AllowWebsocketUpgrade: route.AllowWebsocketUpgrade,
		BlockExploits:         route.BlockExploits,
		HTTP2Support:          route.HTTP2Support,
		AdvancedConfig:        advConfig,
		Locations:             npmLocations,
		AccessListID:          npm.ParseAccessListID(route.AccessListID),
		Meta: map[string]interface{}{
			"managed_by":        ManagedByTag,
			"host_id":           s.cfg.HostID,
			"source":            "lxd",
			"instance":          route.Name,
			"interface":         route.Interface,
			"resolution_method": fmt.Sprintf("lxd-socket (%s -> %s)", route.Interface, route.ForwardHost),
			"synced_at":         time.Now().Format(time.RFC3339),
		},
	}

	if matchedHost == nil {
		s.EmitLog(LevelInfo, "npm", fmt.Sprintf("Creating LXD/Incus proxy host for %v -> %s:%d (instance: %s)", route.DomainNames, route.ForwardHost, route.ForwardPort, route.Name), "")
		created, err := s.npmClient.CreateProxyHost(ctx, desiredReq)
		if err != nil {
			s.EmitLog(LevelError, "npm", fmt.Sprintf("Failed to create LXD proxy host for %v", route.DomainNames), err.Error())
			return
		}
		s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Successfully created LXD proxy host ID #%d for %v (instance: %s)", created.ID, created.DomainNames, route.Name), fmt.Sprintf("Target: %s:%d", created.ForwardHost, created.ForwardPort))
		s.saveTrackedLXDProxy(created.ID, route, "active")
	} else {
		isManaged, _, hostOwner, _, _, _ := getManagedInfo(matchedHost)
		if isManaged && hostOwner != "" && hostOwner != s.cfg.HostID {
			s.EmitLog(LevelWarn, "sync", fmt.Sprintf("Domain(s) %v are managed by another host (%s). Skipping LXD update.", route.DomainNames, hostOwner), "")
			return
		}

		needsUpdate, diffReason := isUpdateRequired(matchedHost, desiredReq)
		if !needsUpdate {
			s.saveTrackedLXDProxy(matchedHost.ID, route, "active")
			return
		}

		s.EmitLog(LevelInfo, "npm", fmt.Sprintf("Updating LXD proxy host ID #%d for %v (%s)", matchedHost.ID, route.DomainNames, diffReason), "")
		updated, err := s.npmClient.UpdateProxyHost(ctx, matchedHost.ID, desiredReq)
		if err != nil {
			s.EmitLog(LevelError, "npm", fmt.Sprintf("Failed to update LXD proxy host #%d", matchedHost.ID), err.Error())
			return
		}
		s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Successfully updated LXD proxy host ID #%d for %v", updated.ID, updated.DomainNames), diffReason)
		s.saveTrackedLXDProxy(updated.ID, route, "active")
	}
}

// saveTrackedLXDProxy records an LXD / Incus proxy host into tracking state.
func (s *Syncer) saveTrackedLXDProxy(npmID int, route lxd.LXDRoute, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var npmLocs []npm.ProxyHostLocation
	for _, l := range route.Locations {
		npmLocs = append(npmLocs, npm.ProxyHostLocation{
			Path:           l.Path,
			ForwardScheme:  l.ForwardScheme,
			ForwardHost:    l.ForwardHost,
			ForwardPort:    l.ForwardPort,
			AdvancedConfig: l.AdvancedConfig,
		})
	}

	key := strings.ToLower(route.DomainNames[0])
	s.trackedProxies[key] = &ManagedProxy{
		NPMHostID:        npmID,
		HostID:           s.cfg.HostID,
		ContainerID:      fmt.Sprintf("lxd:%s", route.Name),
		ContainerName:    route.Name,
		Image:            "lxd/container",
		DomainNames:      route.DomainNames,
		ForwardScheme:    route.ForwardScheme,
		ForwardHost:      route.ForwardHost,
		ForwardPort:      route.ForwardPort,
		ResolutionMethod: fmt.Sprintf("lxd-socket (%s -> %s)", route.Interface, route.ForwardHost),
		SSLEnabled:       route.SSLEnabled,
		SSLForced:        route.SSLForced,
		CertificateID:    route.CertificateID,
		Websocket:        route.AllowWebsocketUpgrade,
		BlockExploits:    route.BlockExploits,
		Locations:        npmLocs,
		Source:           "lxd",
		Status:           status,
		LastSynced:       time.Now(),
	}
}

// saveTrackedProxy records an active managed proxy into the local state.
func (s *Syncer) saveTrackedProxy(npmID int, inspect *docker.ContainerInspect, proxyCfg *ContainerProxyConfig, resMethod, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	healthStatus := "n/a"
	if inspect != nil && inspect.State.Health != nil && inspect.State.Health.Status != "" {
		healthStatus = inspect.State.Health.Status
	}

	image := ""
	if inspect != nil && inspect.Config.Image != "" {
		image = inspect.Config.Image
	}

	var npmLocs []npm.ProxyHostLocation
	for _, l := range proxyCfg.Locations {
		npmLocs = append(npmLocs, npm.ProxyHostLocation{
			Path:           l.Path,
			ForwardScheme:  l.ForwardScheme,
			ForwardHost:    l.ForwardHost,
			ForwardPort:    l.ForwardPort,
			AdvancedConfig: l.AdvancedConfig,
		})
	}

	key := strings.ToLower(proxyCfg.DomainNames[0])
	s.trackedProxies[key] = &ManagedProxy{
		NPMHostID:        npmID,
		HostID:           s.cfg.HostID,
		ContainerID:      proxyCfg.ContainerID,
		ContainerName:    proxyCfg.ContainerName,
		Image:            image,
		DomainNames:      proxyCfg.DomainNames,
		ForwardScheme:    proxyCfg.ForwardScheme,
		ForwardHost:      proxyCfg.ForwardHost,
		ForwardPort:      proxyCfg.ForwardPort,
		ResolutionMethod: resMethod,
		SSLEnabled:       proxyCfg.SSLEnabled,
		SSLForced:        proxyCfg.SSLForced,
		CertificateID:    proxyCfg.CertificateID,
		Websocket:        proxyCfg.AllowWebsocketUpgrade,
		BlockExploits:    proxyCfg.BlockExploits,
		HealthStatus:     healthStatus,
		Locations:        npmLocs,
		Status:           status,
		LastSynced:       time.Now(),
	}
}

// saveTrackedStream records an active Layer 4 TCP/UDP stream into local tracking state.
func (s *Syncer) saveTrackedStream(npmID int, req *npm.StreamRequest, cID, cName, source, sourceFile, resMethod, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.trackedStreams[req.IncomingPort] = &ManagedStream{
		NPMStreamID:      npmID,
		HostID:           s.cfg.HostID,
		ContainerID:      cID,
		ContainerName:    cName,
		IncomingPort:     req.IncomingPort,
		ForwardingHost:   req.ForwardingHost,
		ForwardingPort:   req.ForwardingPort,
		TCPForwarding:    req.TCPForwarding,
		UDPForwarding:    req.UDPForwarding,
		ResolutionMethod: resMethod,
		Source:           source,
		SourceFile:       sourceFile,
		Status:           status,
		LastSynced:       time.Now(),
	}
}

// removeTrackedStream removes a stream from local tracking.
func (s *Syncer) removeTrackedStream(incomingPort int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.trackedStreams, incomingPort)
}

// GetTrackedStreams returns a slice of currently tracked Layer 4 streams.
func (s *Syncer) GetTrackedStreams() []*ManagedStream {
	s.mu.RLock()
	streams := make([]*ManagedStream, 0, len(s.trackedStreams))
	for _, st := range s.trackedStreams {
		streams = append(streams, st)
	}
	s.mu.RUnlock()

	if s.clusterRegistry != nil {
		remote := s.clusterRegistry.GetRemoteStreams()
		streams = append(streams, remote...)
	}

	sort.Slice(streams, func(i, j int) bool {
		return streams[i].IncomingPort < streams[j].IncomingPort
	})

	return streams
}

// removeTrackedProxy deletes a proxy from local tracking.
func (s *Syncer) removeTrackedProxy(containerID string, domainNames []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, d := range domainNames {
		delete(s.trackedProxies, strings.ToLower(d))
	}
	for k, v := range s.trackedProxies {
		if v.ContainerID == containerID {
			delete(s.trackedProxies, k)
		}
	}
}

// syncManualProxiesFromNPM queries NPM for proxy hosts that are unmanaged (configured manually in the NPM portal)
// and updates the local manualProxies registry and container linkages.
func (s *Syncer) syncManualProxiesFromNPM(ctx context.Context) {
	if s.npmClient == nil {
		return
	}

	existingHosts, err := s.npmClient.GetProxyHosts(ctx)
	if err != nil {
		return
	}

	// Fetch all local containers for correlation (including stopped ones)
	var localContainers []docker.ContainerSummary
	if s.dockerClient != nil {
		if cList, err := s.dockerClient.ListContainers(ctx, true); err == nil {
			localContainers = cList
		}
	}

	newManual := make(map[int]*ManagedProxy)
	for _, host := range existingHosts {
		isManaged, _, _, _, _, _ := getManagedInfo(&host)
		if isManaged {
			continue
		}

		sslEnabled := host.CertificateID != nil && fmt.Sprintf("%v", host.CertificateID) != "0" && fmt.Sprintf("%v", host.CertificateID) != ""
		status := "active"
		if !bool(host.Enabled) {
			status = "disabled"
		}

		matchedCID := ""
		matchedCName := ""
		matchedImage := ""
		resMethod := "NPM Portal (Manual)"

		for _, c := range localContainers {
			cNameClean := ""
			if len(c.Names) > 0 {
				cNameClean = strings.TrimPrefix(c.Names[0], "/")
			}
			isMatch := false
			if cNameClean != "" && strings.EqualFold(host.ForwardHost, cNameClean) {
				isMatch = true
			} else if strings.HasPrefix(c.ID, host.ForwardHost) || (len(host.ForwardHost) >= 12 && strings.HasPrefix(host.ForwardHost, c.ID[:min(12, len(c.ID))])) {
				isMatch = true
			}
			if isMatch {
				matchedCID = c.ID
				matchedCName = cNameClean
				matchedImage = c.Image
				resMethod = fmt.Sprintf("NPM Portal (Manual - Linked to '%s')", cNameClean)
				break
			}
		}

		if matchedCName == "" {
			matchedCName = "npm-portal"
			matchedImage = "manual:npm-portal"
		}

		newManual[host.ID] = &ManagedProxy{
			NPMHostID:        host.ID,
			HostID:           s.cfg.HostID,
			ContainerID:      matchedCID,
			ContainerName:    matchedCName,
			Image:            matchedImage,
			DomainNames:      host.DomainNames,
			ForwardScheme:    host.ForwardScheme,
			ForwardHost:      host.ForwardHost,
			ForwardPort:      host.ForwardPort,
			ResolutionMethod: resMethod,
			SSLEnabled:       sslEnabled,
			SSLForced:        bool(host.SSLForced),
			CertificateID:    host.CertificateID,
			Websocket:        bool(host.AllowWebsocketUpgrade),
			BlockExploits:    bool(host.BlockExploits),
			Locations:        host.Locations,
			Source:           "manual",
			Status:           status,
			LastSynced:       time.Now(),
		}
	}

	s.mu.Lock()
	s.manualProxies = newManual
	s.mu.Unlock()
}

// GetTrackedProxies returns a slice of currently tracked auto-discovered proxies and manual NPM portal proxies.
func (s *Syncer) GetTrackedProxies() []*ManagedProxy {
	s.mu.RLock()
	if len(s.manualProxies) == 0 && s.npmClient != nil {
		s.mu.RUnlock()
		s.syncManualProxiesFromNPM(context.Background())
		s.mu.RLock()
	}

	proxies := make([]*ManagedProxy, 0, len(s.trackedProxies)+len(s.manualProxies))
	for _, p := range s.trackedProxies {
		proxies = append(proxies, p)
	}
	for _, p := range s.manualProxies {
		proxies = append(proxies, p)
	}
	s.mu.RUnlock()

	if s.clusterRegistry != nil {
		remote := s.clusterRegistry.GetRemoteProxies()
		proxies = append(proxies, remote...)
	}

	sort.Slice(proxies, func(i, j int) bool {
		if len(proxies[i].DomainNames) > 0 && len(proxies[j].DomainNames) > 0 {
			return proxies[i].DomainNames[0] < proxies[j].DomainNames[0]
		}
		return proxies[i].ContainerName < proxies[j].ContainerName
	})

	return proxies
}

// GetStatusOverview calculates overall system health and counters.
func (s *Syncer) GetStatusOverview(ctx context.Context) StatusOverview {
	s.mu.RLock()
	dockerConn := false
	dockerSocket := ""
	if s.dockerClient != nil {
		dockerConn = s.dockerClient.IsConnected()
		dockerSocket = s.dockerClient.SocketPath()
	}

	npmConn := false
	npmURL := ""
	if s.npmClient != nil {
		npmConn, _, npmURL = s.npmClient.Status()
	}

	activeCount := len(s.trackedProxies) + len(s.manualProxies)
	activeStreamsCount := len(s.trackedStreams)
	totEvents := s.totalEvents
	lastSync := s.lastFullSync
	upSec := int64(time.Since(s.startTime).Seconds())
	dVer := s.dockerVersion
	hID := s.cfg.HostID
	hIP := s.cfg.HostIP
	s.mu.RUnlock()

	runningCount := 0
	if s.dockerClient != nil {
		if containers, err := s.dockerClient.ListContainers(ctx, false); err == nil {
			runningCount = len(containers)
		}
	}

	// PVE status
	pveEnabled := s.cfg.PVEEnabled
	pveConn := false
	pveURL := ""
	pveNode := ""
	pveVer := ""
	pveRunningCount := 0

	s.mu.RLock()
	if len(s.pveClients) > 0 {
		connectedCount := 0
		var urls []string
		for _, cl := range s.pveClients {
			if cl != nil {
				if cl.IsConnected() {
					pveConn = true
					connectedCount++
					if pveVer == "" {
						pveVer = cl.Version()
					}
					if pveNode == "" {
						pveNode = cl.ConfiguredNode()
					}
				}
				urls = append(urls, cl.BaseURL())
			}
		}
		if len(urls) > 0 {
			pveURL = strings.Join(urls, ", ")
		}
		if len(s.pveClients) > 1 {
			pveVer = fmt.Sprintf("%d/%d online", connectedCount, len(s.pveClients))
		}
	} else if s.pveClient != nil {
		pveConn = s.pveClient.IsConnected()
		pveURL = s.pveClient.BaseURL()
		pveNode = s.pveClient.ConfiguredNode()
		pveVer = s.pveClient.Version()
	}
	for _, c := range s.pveContainers {
		if strings.EqualFold(c.Status, "running") {
			pveRunningCount++
		}
	}
	s.mu.RUnlock()

	// LXD status
	lxdEnabled := s.cfg.LXDEnabled
	lxdConn := false
	lxdSocket := ""
	lxdServerType := ""
	lxdVer := ""
	lxdRunningCount := 0
	if s.lxdClient != nil {
		lxdConn = s.lxdClient.IsConnected()
		lxdSocket = s.lxdClient.SocketPath()
		lxdServerType = s.lxdClient.ServerType()
		lxdVer = s.lxdClient.ServerVersion()
		s.mu.RLock()
		for _, inst := range s.lxdInstances {
			if strings.EqualFold(inst.Status, "running") || inst.StatusCode == 103 {
				lxdRunningCount++
			}
		}
		s.mu.RUnlock()
	}

	clusterNodesCount := 1
	clusterOnlineCount := 1
	if s.clusterRegistry != nil {
		nodes := s.clusterRegistry.GetNodes(ClusterNodeInfo{NodeID: hID, Status: "online"})
		clusterNodesCount = len(nodes)
		clusterOnlineCount = 0
		for _, n := range nodes {
			if n.Status == "online" {
				clusterOnlineCount++
			}
		}
	}

	return StatusOverview{
		HostID:               hID,
		HostIP:               hIP,
		DockerConnected:     dockerConn,
		DockerSocket:        dockerSocket,
		DockerVersion:       dVer,
		NPMConnected:        npmConn,
		NPMURL:              npmURL,
		ActiveProxies:       activeCount,
		ActiveStreams:       activeStreamsCount,
		RunningContainers:   runningCount,
		TotalEvents:         totEvents,
		LastFullSync:        lastSync,
		Uptime:              upSec,
		PVEEnabled:           pveEnabled,
		PVEConnected:         pveConn,
		PVEURL:               pveURL,
		PVENode:              pveNode,
		PVEVersion:           pveVer,
		PVERunningContainers: pveRunningCount,
		LXDEnabled:           lxdEnabled,
		LXDConnected:         lxdConn,
		LXDSocket:            lxdSocket,
		LXDServerType:        lxdServerType,
		LXDVersion:           lxdVer,
		LXDRunningContainers: lxdRunningCount,
		ClusterNodesCount:    clusterNodesCount,
		ClusterOnlineNodes:   clusterOnlineCount,
	}
}

// getLocalContainersView lists all running containers on this local host (Docker, Proxmox LXC, LXD/Incus).
func (s *Syncer) getLocalContainersView(ctx context.Context) ([]ContainerStatusView, error) {
	views := make([]ContainerStatusView, 0)
	localID := s.cfg.HostID
	if localID == "" {
		localID = "controller-main"
	}

	// 1. Docker Containers
	if s.dockerClient != nil {
		containers, err := s.dockerClient.ListContainers(ctx, true)
		if err == nil {
			for _, c := range containers {
				name := ""
				if len(c.Names) > 0 {
					name = strings.TrimPrefix(c.Names[0], "/")
				}

				inspect, err := s.dockerClient.InspectContainer(ctx, c.ID)
				if err != nil {
					views = append(views, ContainerStatusView{
						ID:            c.ID[:min(12, len(c.ID))],
						Name:          name,
						Image:         c.Image,
						Source:        "docker",
						State:         c.State,
						Status:        c.Status,
						Discovered:    false,
						IgnoredReason: "Failed to inspect container",
						Labels:        c.Labels,
						NodeID:        localID,
					})
					continue
				}

				proxyCfg, shouldProxy, parseErr := ParseContainerLabels(inspect, s.cfg)
				discovered := shouldProxy && proxyCfg != nil
				manualNPM := false
				ignoredReason := ""

				var domains []string
				port := 0
				if discovered {
					domains = proxyCfg.DomainNames
					port = proxyCfg.ForwardPort
				} else if parseErr != nil {
					ignoredReason = parseErr.Error()
				} else {
					ignoredReason = "Missing discovery labels ('npm.frontend.domain')"
				}

				// Check if this container is manually configured via NPM Portal
				if !discovered {
					s.mu.RLock()
					if len(s.manualProxies) == 0 && s.npmClient != nil {
						s.mu.RUnlock()
						s.syncManualProxiesFromNPM(ctx)
						s.mu.RLock()
					}

					var manualDomains []string
					manualPort := 0
					hasManual := false
					for _, mp := range s.manualProxies {
						isMatch := false
						if mp.ContainerID != "" && (mp.ContainerID == c.ID || strings.HasPrefix(c.ID, mp.ContainerID)) {
							isMatch = true
						} else if name != "" && strings.EqualFold(mp.ForwardHost, name) {
							isMatch = true
						} else if mp.ContainerName != "" && name != "" && strings.EqualFold(mp.ContainerName, name) {
							isMatch = true
						} else {
							if inspect.NetworkSettings.IPAddress != "" && mp.ForwardHost == inspect.NetworkSettings.IPAddress {
								isMatch = true
							}
							if !isMatch && (mp.ForwardHost == "localhost" || mp.ForwardHost == "127.0.0.1" || mp.ForwardHost == s.cfg.HostIP) {
								for _, bindings := range inspect.NetworkSettings.Ports {
									for _, b := range bindings {
										if hp, err := strconv.Atoi(b.HostPort); err == nil && hp == mp.ForwardPort {
											isMatch = true
											break
										}
									}
									if isMatch {
										break
									}
								}
							}
						}

						if isMatch {
							hasManual = true
							manualDomains = append(manualDomains, mp.DomainNames...)
							if mp.ForwardPort > 0 && manualPort == 0 {
								manualPort = mp.ForwardPort
							}
						}
					}
					s.mu.RUnlock()

					if hasManual {
						discovered = true
						manualNPM = true
						ignoredReason = "Manually configured from NPM portal"
						domains = manualDomains
						if port == 0 {
							port = manualPort
						}
					}
				}

				healthStatus := "none"
				if inspect.State.Health != nil && inspect.State.Health.Status != "" {
					healthStatus = inspect.State.Health.Status
				}

				views = append(views, ContainerStatusView{
					ID:            c.ID[:min(12, len(c.ID))],
					Name:          name,
					Image:         c.Image,
					Source:        "docker",
					State:         c.State,
					Status:        c.Status,
					HealthStatus:  healthStatus,
					Discovered:    discovered,
					ManualNPM:     manualNPM,
					IgnoredReason: ignoredReason,
					Domains:       domains,
					Port:          port,
					Labels:        c.Labels,
					NodeID:        localID,
				})
			}
		}
	}

	// 2. Proxmox VE LXC Containers
	s.mu.RLock()
	pveSnapshot := make([]pve.LXCContainerSummary, len(s.pveContainers))
	copy(pveSnapshot, s.pveContainers)
	lxdSnapshot := make([]lxd.Instance, len(s.lxdInstances))
	copy(lxdSnapshot, s.lxdInstances)
	s.mu.RUnlock()

	for _, pc := range pveSnapshot {
		tagsMeta := pve.ParsePVETags(pc.Tags)
		if pc.Notes != "" {
			notesMeta := pve.ParsePVENotes(pc.Notes)
			for k, v := range notesMeta {
				if _, exists := tagsMeta[k]; !exists {
					tagsMeta[k] = v
				}
			}
		}

		discovered := false
		var domains []string
		port := 80
		if d, ok := tagsMeta["npm.domain"]; ok && d != "" {
			discovered = true
			domains = strings.Split(d, ",")
		}
		if pStr, ok := tagsMeta["npm.port"]; ok {
			if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
				port = p
			}
		}

		ignored := ""
		if !discovered {
			ignored = "No Proxmox discovery tags ('npm-domain=...') or notes"
		}

		pveID := fmt.Sprintf("pve:%d", pc.VMID)
		if pc.Node != "" {
			pveID = fmt.Sprintf("pve:%s:%d", pc.Node, pc.VMID)
		}

		pveType := strings.ToLower(pc.Type)
		if pveType == "" {
			pveType = "lxc"
		}

		displayName := pc.Name
		if displayName == "" {
			displayName = fmt.Sprintf("VM %d", pc.VMID)
		}
		if pc.Node != "" {
			displayName = fmt.Sprintf("%s (%s)", displayName, pc.Node)
		}

		statusDetail := pc.Status
		if pc.IP != "" {
			statusDetail = fmt.Sprintf("%s • %s", pc.Status, pc.IP)
		}

		views = append(views, ContainerStatusView{
			ID:            pveID,
			Name:          displayName,
			Image:         pveType,
			Source:        "pve",
			State:         pc.Status,
			Status:        statusDetail,
			HealthStatus:  "none",
			Discovered:    discovered,
			IgnoredReason: ignored,
			Domains:       domains,
			Port:          port,
			Labels:        tagsMeta,
			NodeID:        localID,
		})
	}

	// 3. Canonical LXD / Incus Instances
	for _, li := range lxdSnapshot {
		tagsMeta := make(map[string]string)
		for k, v := range li.Config {
			if strings.HasPrefix(k, "user.npm.") {
				tagsMeta[k] = v
			}
		}
		discovered := false
		var domains []string
		port := 80
		if d, ok := tagsMeta["user.npm.domain"]; ok && d != "" {
			discovered = true
			domains = strings.Split(d, ",")
		}
		if pStr, ok := tagsMeta["user.npm.port"]; ok {
			if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
				port = p
			}
		}

		ignored := ""
		if !discovered {
			ignored = "No LXD discovery metadata ('user.npm.domain')"
		}

		views = append(views, ContainerStatusView{
			ID:            fmt.Sprintf("lxd:%s", li.Name),
			Name:          li.Name,
			Image:         li.Type,
			Source:        "lxd",
			State:         li.Status,
			Status:        li.Status,
			HealthStatus:  "none",
			Discovered:    discovered,
			IgnoredReason: ignored,
			Domains:       domains,
			Port:          port,
			Labels:        tagsMeta,
			NodeID:        localID,
		})
	}

	return views, nil
}

// GetContainersView lists all running containers across Docker, Proxmox LXC, and LXD/Incus, including remote cluster nodes.
func (s *Syncer) GetContainersView(ctx context.Context) ([]ContainerStatusView, error) {
	views, err := s.getLocalContainersView(ctx)
	if err != nil {
		views = make([]ContainerStatusView, 0)
	}

	if s.clusterRegistry != nil {
		remote := s.clusterRegistry.GetRemoteContainers()
		views = append(views, remote...)
	}

	return views, nil
}

// EmitLog appends a log event to memory history and broadcasts it to live SSE subscribers.
func (s *Syncer) EmitLog(level LogLevel, category, message, details string) {
	s.mu.Lock()
	s.logSeq++
	event := LogEvent{
		ID:        s.logSeq,
		Timestamp: time.Now(),
		Level:     level,
		Category:  category,
		Message:   message,
		Details:   details,
		NodeID:    s.cfg.HostID,
	}

	if len(s.logEvents) >= MaxLogsHistory {
		s.logEvents = s.logEvents[1:]
	}
	s.logEvents = append(s.logEvents, event)

	// Broadcast to active subscribers
	subscribers := make([]chan LogEvent, 0, len(s.logSubscribers))
	for ch := range s.logSubscribers {
		subscribers = append(subscribers, ch)
	}
	s.mu.Unlock()

	// Print to console stdout
	log.Printf("[%s] [%s] %s %s", level, category, message, details)

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
			// Drop if subscriber channel is blocked
		}
	}
}

// EmitExternalLog broadcasts a log event received from a remote worker node.
func (s *Syncer) EmitExternalLog(event LogEvent) {
	s.mu.Lock()
	s.logSeq++
	event.ID = s.logSeq
	if len(s.logEvents) >= MaxLogsHistory {
		s.logEvents = s.logEvents[1:]
	}
	s.logEvents = append(s.logEvents, event)

	subscribers := make([]chan LogEvent, 0, len(s.logSubscribers))
	for ch := range s.logSubscribers {
		subscribers = append(subscribers, ch)
	}
	s.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
			// Drop if subscriber channel is blocked
		}
	}
}

// GetRecentEventsSince returns log events that occurred after the given timestamp.
func (s *Syncer) GetRecentEventsSince(since time.Time) []LogEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []LogEvent
	for _, ev := range s.logEvents {
		if ev.Timestamp.After(since) {
			result = append(result, ev)
		}
	}
	return result
}


// GetLogEvents returns recent log history.
func (s *Syncer) GetLogEvents(limit int) []LogEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.logEvents) {
		limit = len(s.logEvents)
	}

	result := make([]LogEvent, limit)
	start := len(s.logEvents) - limit
	copy(result, s.logEvents[start:])
	return result
}

// SubscribeLogs adds a channel to receive live log events.
func (s *Syncer) SubscribeLogs() (chan LogEvent, func()) {
	ch := make(chan LogEvent, 100)
	s.mu.Lock()
	s.logSubscribers[ch] = struct{}{}
	s.mu.Unlock()

	unsubscribe := func() {
		s.mu.Lock()
		delete(s.logSubscribers, ch)
		close(ch)
		s.mu.Unlock()
	}

	return ch, unsubscribe
}

// Helper: checks if two domain lists overlap
func domainsOverlap(a, b []string) bool {
	set := make(map[string]bool)
	for _, d := range a {
		set[strings.ToLower(strings.TrimSpace(d))] = true
	}
	for _, d := range b {
		if set[strings.ToLower(strings.TrimSpace(d))] {
			return true
		}
	}
	return false
}

// Helper: checks if a proxy host in NPM is managed by a given host and container
func (s *Syncer) isManagedByThisHostAndContainer(host *npm.ProxyHost, containerID, containerName string) bool {
	isManaged, source, hostOwner, cID, cName, _ := getManagedInfo(host)
	if !isManaged || source == "iac" {
		return false
	}
	// Multi-host safety check: only match if owned by this host (or unowned legacy)
	if hostOwner != "" && hostOwner != s.cfg.HostID {
		return false
	}
	if cID != "" && (cID == containerID || strings.HasPrefix(containerID, cID) || strings.HasPrefix(cID, containerID)) {
		return true
	}
	if cName != "" && (cName == containerName || strings.TrimPrefix(containerName, "/") == cName) {
		return true
	}
	return false
}

// Helper: extracts managed_by metadata, source (docker/iac), host ID, and container ID/name from NPM proxy host
func getManagedInfo(host *npm.ProxyHost) (isManaged bool, source, hostID, containerID, containerName, sourceFile string) {
	source = "docker" // default source
	if host.Meta != nil {
		if mb, ok := host.Meta["managed_by"].(string); ok && mb == ManagedByTag {
			isManaged = true
			if src, ok := host.Meta["source"].(string); ok && src != "" {
				source = src
			}
			if hid, ok := host.Meta["host_id"].(string); ok {
				hostID = hid
			}
			if id, ok := host.Meta["container_id"].(string); ok {
				containerID = id
			}
			if name, ok := host.Meta["container_name"].(string); ok {
				containerName = name
			}
			if sf, ok := host.Meta["source_file"].(string); ok {
				sourceFile = sf
			}
			return
		}
	}
	// Fallback to checking HeaderComment in advanced_config
	if strings.Contains(host.AdvancedConfig, HeaderComment) {
		isManaged = true
		if strings.Contains(host.AdvancedConfig, "[source: iac]") {
			source = "iac"
		} else if strings.Contains(host.AdvancedConfig, "[source: pve]") {
			source = "pve"
		} else if strings.Contains(host.AdvancedConfig, "[source: lxd]") {
			source = "lxd"
		}
		// Extract [host_id: ...] if present
		if idx := strings.Index(host.AdvancedConfig, "[host_id: "); idx != -1 {
			rem := host.AdvancedConfig[idx+len("[host_id: "):]
			if endIdx := strings.Index(rem, "]"); endIdx != -1 {
				hostID = strings.TrimSpace(rem[:endIdx])
			}
		}
	}
	return
}

// normalizeID safely normalizes numeric/string IDs (like certificate_id and access_list_id) for comparison
func normalizeID(v interface{}) string {
	if v == nil {
		return "0"
	}
	switch val := v.(type) {
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.Itoa(int(val))
	case string:
		s := strings.TrimSpace(val)
		if s == "" || s == "0" {
			return "0"
		}
		return s
	default:
		s := strings.TrimSpace(fmt.Sprintf("%v", val))
		if s == "" || s == "0" || s == "<nil>" {
			return "0"
		}
		return s
	}
}

// Helper: checks if an update is required between NPM's existing proxy host and desired config
func isUpdateRequired(existing *npm.ProxyHost, desired *npm.ProxyHostRequest) (bool, string) {
	if !equalStringSlices(existing.DomainNames, desired.DomainNames) {
		return true, "domain names changed"
	}
	if existing.ForwardHost != desired.ForwardHost {
		return true, fmt.Sprintf("target host changed from %s to %s", existing.ForwardHost, desired.ForwardHost)
	}
	if existing.ForwardPort != desired.ForwardPort {
		return true, fmt.Sprintf("target port changed from %d to %d", existing.ForwardPort, desired.ForwardPort)
	}
	if !strings.EqualFold(existing.ForwardScheme, desired.ForwardScheme) {
		return true, fmt.Sprintf("forward scheme changed from %s to %s", existing.ForwardScheme, desired.ForwardScheme)
	}
	if normalizeID(existing.CertificateID) != normalizeID(desired.CertificateID) {
		return true, fmt.Sprintf("certificate ID changed from %v to %v", existing.CertificateID, desired.CertificateID)
	}
	if bool(existing.SSLForced) != desired.SSLForced {
		return true, "SSL forced setting changed"
	}
	if bool(existing.AllowWebsocketUpgrade) != desired.AllowWebsocketUpgrade {
		return true, "websocket upgrade setting changed"
	}
	if bool(existing.BlockExploits) != desired.BlockExploits {
		return true, "block exploits setting changed"
	}
	if bool(existing.CachingEnabled) != desired.CachingEnabled {
		return true, "caching setting changed"
	}
	if bool(existing.HTTP2Support) != desired.HTTP2Support {
		return true, "http2 support setting changed"
	}
	if bool(existing.HSTSEnabled) != desired.HSTSEnabled {
		return true, "HSTS setting changed"
	}
	if bool(existing.HSTSSubdomains) != desired.HSTSSubdomains {
		return true, "HSTS subdomains setting changed"
	}
	if normalizeID(existing.AccessListID) != normalizeID(desired.AccessListID) {
		return true, fmt.Sprintf("access list ID changed from %v to %v", existing.AccessListID, desired.AccessListID)
	}
	if !equalLocations(existing.Locations, desired.Locations) {
		return true, "custom locations changed"
	}
	if strings.TrimSpace(existing.AdvancedConfig) != strings.TrimSpace(desired.AdvancedConfig) {
		return true, "advanced Nginx config changed"
	}

	return false, ""
}

func equalLocations(existing, desired []npm.ProxyHostLocation) bool {
	if len(existing) != len(desired) {
		return false
	}
	existMap := make(map[string]npm.ProxyHostLocation)
	for _, l := range existing {
		existMap[l.Path] = l
	}
	for _, d := range desired {
		e, ok := existMap[d.Path]
		if !ok {
			return false
		}
		if !strings.EqualFold(e.ForwardScheme, d.ForwardScheme) ||
			e.ForwardHost != d.ForwardHost ||
			e.ForwardPort != d.ForwardPort ||
			strings.TrimSpace(e.AdvancedConfig) != strings.TrimSpace(d.AdvancedConfig) {
			return false
		}
	}
	return true
}

func getStreamManagedInfo(stream *npm.Stream) (isManaged bool, source, hostID, containerID string) {
	source = "docker"
	if stream.Meta != nil {
		if mb, ok := stream.Meta["managed_by"].(string); ok && mb == ManagedByTag {
			isManaged = true
			if src, ok := stream.Meta["source"].(string); ok && src != "" {
				source = src
			}
			if hid, ok := stream.Meta["host_id"].(string); ok {
				hostID = hid
			}
			if cid, ok := stream.Meta["container_id"].(string); ok {
				containerID = cid
			}
			return
		}
	}
	return
}

func isStreamUpdateRequired(existing *npm.Stream, desired *npm.StreamRequest) (bool, string) {
	if existing.IncomingPort != desired.IncomingPort {
		return true, fmt.Sprintf("incoming port changed from %d to %d", existing.IncomingPort, desired.IncomingPort)
	}
	if existing.ForwardingHost != desired.ForwardingHost {
		return true, fmt.Sprintf("forwarding host changed from %s to %s", existing.ForwardingHost, desired.ForwardingHost)
	}
	if existing.ForwardingPort != desired.ForwardingPort {
		return true, fmt.Sprintf("forwarding port changed from %d to %d", existing.ForwardingPort, desired.ForwardingPort)
	}
	if bool(existing.TCPForwarding) != desired.TCPForwarding {
		return true, "TCP forwarding setting changed"
	}
	if bool(existing.UDPForwarding) != desired.UDPForwarding {
		return true, "UDP forwarding setting changed"
	}
	return false, ""
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ca := make([]string, len(a))
	cb := make([]string, len(b))
	copy(ca, a)
	copy(cb, b)
	sort.Strings(ca)
	sort.Strings(cb)
	return reflect.DeepEqual(ca, cb)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetNodePVEConfigs returns all Proxmox VE configurations for a given node.
func (s *Syncer) GetNodePVEConfigs(nodeID string) []PVEConfig {
	localID := s.cfg.HostID
	if localID == "" {
		localID = "controller-main"
	}

	if nodeID == "" || strings.EqualFold(nodeID, localID) || strings.EqualFold(nodeID, "local") || strings.EqualFold(nodeID, "controller-main") {
		if s.clusterRegistry != nil {
			if cfgs := s.clusterRegistry.GetNodePVEConfigs(localID); len(cfgs) > 0 {
				return cfgs
			}
		}

		s.mu.RLock()
		defer s.mu.RUnlock()
		if len(s.pveConfigs) > 0 {
			res := make([]PVEConfig, 0, len(s.pveConfigs))
			for _, c := range s.pveConfigs {
				cp := c
				cp.HasSecret = cp.TokenSecret != ""
				res = append(res, cp)
			}
			return res
		}

		if s.cfg.PVEURL != "" {
			return []PVEConfig{{
				ID:                 "default",
				Name:               "Default Proxmox",
				Enabled:            s.cfg.PVEEnabled,
				URL:                s.cfg.PVEURL,
				TokenID:            s.cfg.PVETokenID,
				TokenSecret:        s.cfg.PVETokenSecret,
				Node:               s.cfg.PVENode,
				VerifySSL:          s.cfg.PVEVerifySSL,
				PreferredInterface: s.cfg.PVEPreferredInterface,
				AllowedSubnets:     s.cfg.PVEAllowedSubnets,
				HasSecret:          s.cfg.PVETokenSecret != "",
			}}
		}
		return nil
	}

	if s.clusterRegistry != nil {
		return s.clusterRegistry.GetNodePVEConfigs(nodeID)
	}
	return nil
}

// GetNodePVEConfig returns the primary Proxmox VE configuration for a given node.
func (s *Syncer) GetNodePVEConfig(nodeID string) *PVEConfig {
	cfgs := s.GetNodePVEConfigs(nodeID)
	if len(cfgs) == 0 {
		return nil
	}
	for _, c := range cfgs {
		if c.Enabled {
			cp := c
			return &cp
		}
	}
	cp := cfgs[0]
	return &cp
}

// GetNodePVEConfigByID returns a specific Proxmox VE configuration by ID for a given node.
func (s *Syncer) GetNodePVEConfigByID(nodeID string, configID string) *PVEConfig {
	cfgs := s.GetNodePVEConfigs(nodeID)
	for _, c := range cfgs {
		if c.ID == configID || (configID == "default" && (c.ID == "default" || c.ID == "")) {
			cp := c
			return &cp
		}
	}
	return nil
}

// SetNodePVEConfig updates or adds a Proxmox VE configuration for a specific node (controller or remote worker).
func (s *Syncer) SetNodePVEConfig(nodeID string, cfg PVEConfig) error {
	localID := s.cfg.HostID
	if localID == "" {
		localID = "controller-main"
	}

	if nodeID == "" || strings.EqualFold(nodeID, localID) || strings.EqualFold(nodeID, "local") || strings.EqualFold(nodeID, "controller-main") {
		if s.clusterRegistry != nil {
			s.clusterRegistry.SetNodePVEConfig(localID, cfg)
			allCfgs := s.clusterRegistry.GetNodePVEConfigs(localID)
			return s.ApplyDynamicPVEConfigs(allCfgs)
		}
		return s.ApplyDynamicPVEConfig(cfg)
	}

	if s.clusterRegistry != nil {
		s.clusterRegistry.SetNodePVEConfig(nodeID, cfg)
		s.EmitLog(LevelInfo, "cluster", fmt.Sprintf("Proxmox VE configuration updated for remote worker '%s'", nodeID),
			fmt.Sprintf("Enabled: %v, URL: %s", cfg.Enabled, cfg.URL))
		return nil
	}

	return fmt.Errorf("cluster registry unavailable")
}

// SetNodePVEConfigs replaces all Proxmox VE configurations for a specific node.
func (s *Syncer) SetNodePVEConfigs(nodeID string, cfgs []PVEConfig) error {
	localID := s.cfg.HostID
	if localID == "" {
		localID = "controller-main"
	}

	if nodeID == "" || strings.EqualFold(nodeID, localID) || strings.EqualFold(nodeID, "local") || strings.EqualFold(nodeID, "controller-main") {
		if s.clusterRegistry != nil {
			s.clusterRegistry.SetNodePVEConfigs(localID, cfgs)
			allCfgs := s.clusterRegistry.GetNodePVEConfigs(localID)
			return s.ApplyDynamicPVEConfigs(allCfgs)
		}
		return s.ApplyDynamicPVEConfigs(cfgs)
	}

	if s.clusterRegistry != nil {
		s.clusterRegistry.SetNodePVEConfigs(nodeID, cfgs)
		s.EmitLog(LevelInfo, "cluster", fmt.Sprintf("Proxmox VE configurations updated for remote worker '%s' (%d endpoints)", nodeID, len(cfgs)), "")
		return nil
	}

	return fmt.Errorf("cluster registry unavailable")
}

// DeleteNodePVEConfig removes a Proxmox VE configuration by ID for a specific node.
func (s *Syncer) DeleteNodePVEConfig(nodeID string, configID string) error {
	localID := s.cfg.HostID
	if localID == "" {
		localID = "controller-main"
	}

	if nodeID == "" || strings.EqualFold(nodeID, localID) || strings.EqualFold(nodeID, "local") || strings.EqualFold(nodeID, "controller-main") {
		if s.clusterRegistry != nil {
			s.clusterRegistry.DeleteNodePVEConfig(localID, configID)
			allCfgs := s.clusterRegistry.GetNodePVEConfigs(localID)
			return s.ApplyDynamicPVEConfigs(allCfgs)
		}
		s.mu.Lock()
		delete(s.pveClients, configID)
		delete(s.pveConfigs, configID)
		var first *pve.Client
		for _, cl := range s.pveClients {
			first = cl
			break
		}
		s.pveClient = first
		s.cfg.PVEEnabled = len(s.pveClients) > 0
		s.mu.Unlock()
		go s.TriggerSync()
		return nil
	}

	if s.clusterRegistry != nil {
		if s.clusterRegistry.DeleteNodePVEConfig(nodeID, configID) {
			s.EmitLog(LevelInfo, "cluster", fmt.Sprintf("Proxmox VE configuration '%s' removed from remote worker '%s'", configID, nodeID), "")
			return nil
		}
		return fmt.Errorf("config '%s' not found for node '%s'", configID, nodeID)
	}

	return fmt.Errorf("cluster registry unavailable")
}

// ApplyDynamicPVEConfigs applies multiple Proxmox VE configurations at runtime.
func (s *Syncer) ApplyDynamicPVEConfigs(cfgs []PVEConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pveClients == nil {
		s.pveClients = make(map[string]*pve.Client)
	}
	if s.pveConfigs == nil {
		s.pveConfigs = make(map[string]PVEConfig)
	}

	newClients := make(map[string]*pve.Client)
	newConfigs := make(map[string]PVEConfig)
	var firstClient *pve.Client
	var primaryCfg *PVEConfig
	anyEnabled := false

	for i, cfg := range cfgs {
		if cfg.ID == "" {
			cfg.ID = fmt.Sprintf("pve-%d", i+1)
		}
		newConfigs[cfg.ID] = cfg

		if !cfg.Enabled || cfg.URL == "" {
			continue
		}
		anyEnabled = true
		if primaryCfg == nil {
			primaryCfg = &cfg
		}

		secret := cfg.TokenSecret
		if secret == "" {
			if oldCfg, ok := s.pveConfigs[cfg.ID]; ok && oldCfg.TokenSecret != "" {
				secret = oldCfg.TokenSecret
			} else if s.cfg.PVETokenSecret != "" {
				secret = s.cfg.PVETokenSecret
			}
		}

		preferredIface := cfg.PreferredInterface
		if preferredIface == "" {
			preferredIface = "eth0"
		}

		var client *pve.Client
		cleanURL := strings.TrimRight(cfg.URL, "/")
		if !strings.HasPrefix(cleanURL, "http://") && !strings.HasPrefix(cleanURL, "https://") {
			cleanURL = "https://" + cleanURL
		}

		if existing, ok := s.pveClients[cfg.ID]; ok && existing != nil &&
			existing.BaseURL() == cleanURL &&
			existing.ConfiguredNode() == cfg.Node {
			client = existing
		} else {
			var err error
			client, err = pve.NewClient(
				cfg.URL,
				cfg.TokenID,
				secret,
				cfg.Node,
				cfg.VerifySSL,
				10*time.Second,
				preferredIface,
				cfg.AllowedSubnets,
			)
			if err != nil {
				s.EmitLog(LevelError, "pve", fmt.Sprintf("Failed to initialize Proxmox VE client [%s]", cfg.ID), err.Error())
				continue
			}
			go func(c *pve.Client, u string, id string) {
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cancel()
				if err := c.Ping(ctx); err == nil {
					s.EmitLog(LevelSuccess, "pve", fmt.Sprintf("Connected to Proxmox VE API %s [%s]", c.Version(), id), u)
				}
			}(client, cfg.URL, cfg.ID)
		}

		newClients[cfg.ID] = client
		if firstClient == nil {
			firstClient = client
		}
	}

	s.pveClients = newClients
	s.pveConfigs = newConfigs
	s.pveClient = firstClient
	s.cfg.PVEEnabled = anyEnabled

	if primaryCfg != nil {
		s.cfg.PVEURL = primaryCfg.URL
		s.cfg.PVETokenID = primaryCfg.TokenID
		if primaryCfg.TokenSecret != "" {
			s.cfg.PVETokenSecret = primaryCfg.TokenSecret
		}
		s.cfg.PVENode = primaryCfg.Node
		s.cfg.PVEVerifySSL = primaryCfg.VerifySSL
		s.cfg.PVEPreferredInterface = primaryCfg.PreferredInterface
		s.cfg.PVEAllowedSubnets = primaryCfg.AllowedSubnets
	}

	if len(newClients) == 0 {
		s.pveContainers = nil
	}

	go s.TriggerSync()
	return nil
}

// ApplyDynamicPVEConfig applies a single Proxmox VE configuration at runtime.
func (s *Syncer) ApplyDynamicPVEConfig(pveCfg PVEConfig) error {
	s.mu.Lock()
	if s.pveConfigs == nil {
		s.pveConfigs = make(map[string]PVEConfig)
	}
	cfgList := make([]PVEConfig, 0, len(s.pveConfigs)+1)
	found := false
	for _, c := range s.pveConfigs {
		if (pveCfg.ID != "" && c.ID == pveCfg.ID) || (pveCfg.ID == "" && (c.ID == "default" || len(s.pveConfigs) == 1)) {
			cfgList = append(cfgList, pveCfg)
			found = true
		} else {
			cfgList = append(cfgList, c)
		}
	}
	if !found {
		cfgList = append(cfgList, pveCfg)
	}
	s.mu.Unlock()

	return s.ApplyDynamicPVEConfigs(cfgList)
}
