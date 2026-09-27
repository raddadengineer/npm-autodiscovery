package syncer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// NodeReport contains telemetry and state pushed by a remote worker node.
type NodeReport struct {
	NodeID       string                `json:"node_id"`
	NodeIP       string                `json:"node_ip"`
	ReportedAt   time.Time             `json:"reported_at"`
	Status       StatusOverview        `json:"status"`
	Containers   []ContainerStatusView `json:"containers"`
	Proxies      []*ManagedProxy       `json:"proxies"`
	Streams      []*ManagedStream      `json:"streams"`
	RecentEvents []LogEvent            `json:"recent_events"`
}

// ClusterNodeInfo contains aggregated status of a cluster node (controller or remote worker).
type ClusterNodeInfo struct {
	NodeID          string                `json:"node_id"`
	NodeIP          string                `json:"node_ip"`
	IsController    bool                  `json:"is_controller"`
	Status          string                `json:"status"` // "online", "offline"
	LastHeartbeat   time.Time             `json:"last_heartbeat"`
	UptimeSeconds   int64                 `json:"uptime_seconds"`
	DockerConnected bool                  `json:"docker_connected"`
	DockerVersion   string                `json:"docker_version"`
	NPMConnected    bool                  `json:"npm_connected"`
	PVEConnected    bool                  `json:"pve_connected"`
	LXDConnected    bool                  `json:"lxd_connected"`
	ContainerCount  int                   `json:"container_count"`
	ProxyCount      int                   `json:"proxy_count"`
	StreamCount     int                   `json:"stream_count"`
	Overview        StatusOverview        `json:"overview"`
	Containers      []ContainerStatusView `json:"containers,omitempty"`
	Proxies         []*ManagedProxy       `json:"proxies,omitempty"`
	Streams         []*ManagedStream      `json:"streams,omitempty"`
}

// ClusterRegistry stores and coordinates remote worker telemetry on the controller node.
type ClusterRegistry struct {
	mu    sync.RWMutex
	nodes map[string]*ClusterNodeInfo // Key: NodeID
}

// NewClusterRegistry creates an empty registry for tracking cluster nodes.
func NewClusterRegistry() *ClusterRegistry {
	return &ClusterRegistry{
		nodes: make(map[string]*ClusterNodeInfo),
	}
}

// RegisterOrUpdate saves or updates telemetry from a remote worker report.
func (r *ClusterRegistry) RegisterOrUpdate(report NodeReport) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Ensure each container is explicitly stamped with worker's node ID
	taggedContainers := make([]ContainerStatusView, len(report.Containers))
	for i, c := range report.Containers {
		if c.NodeID == "" {
			c.NodeID = report.NodeID
		}
		taggedContainers[i] = c
	}

	// Ensure each proxy is stamped with host ID
	taggedProxies := make([]*ManagedProxy, len(report.Proxies))
	for i, p := range report.Proxies {
		cp := *p
		if cp.HostID == "" {
			cp.HostID = report.NodeID
		}
		taggedProxies[i] = &cp
	}

	// Ensure each stream is stamped with host ID
	taggedStreams := make([]*ManagedStream, len(report.Streams))
	for i, s := range report.Streams {
		cs := *s
		if cs.HostID == "" {
			cs.HostID = report.NodeID
		}
		taggedStreams[i] = &cs
	}

	info := &ClusterNodeInfo{
		NodeID:          report.NodeID,
		NodeIP:          report.NodeIP,
		IsController:    false,
		Status:          "online",
		LastHeartbeat:   time.Now(),
		UptimeSeconds:   report.Status.Uptime,
		DockerConnected: report.Status.DockerConnected,
		DockerVersion:   report.Status.DockerVersion,
		NPMConnected:    report.Status.NPMConnected,
		PVEConnected:    report.Status.PVEConnected,
		LXDConnected:    report.Status.LXDConnected,
		ContainerCount:  len(taggedContainers),
		ProxyCount:      len(taggedProxies),
		StreamCount:     len(taggedStreams),
		Overview:        report.Status,
		Containers:      taggedContainers,
		Proxies:         taggedProxies,
		Streams:         taggedStreams,
	}

	r.nodes[report.NodeID] = info
}

// GetNodes returns all cluster nodes: local controller first, then remote workers sorted by ID.
func (r *ClusterRegistry) GetNodes(localNode ClusterNodeInfo) []ClusterNodeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ClusterNodeInfo, 0, len(r.nodes)+1)
	result = append(result, localNode)

	// Collect remote nodes
	remotes := make([]ClusterNodeInfo, 0, len(r.nodes))
	now := time.Now()
	for _, node := range r.nodes {
		cp := *node
		// Nodes without heartbeat for > 45 seconds are marked offline
		if now.Sub(cp.LastHeartbeat) > 45*time.Second {
			cp.Status = "offline"
		} else {
			cp.Status = "online"
		}
		remotes = append(remotes, cp)
	}

	sort.Slice(remotes, func(i, j int) bool {
		return remotes[i].NodeID < remotes[j].NodeID
	})

	result = append(result, remotes...)
	return result
}

// GetRemoteContainers aggregates containers from all registered worker nodes.
func (r *ClusterRegistry) GetRemoteContainers() []ContainerStatusView {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var all []ContainerStatusView
	for _, node := range r.nodes {
		all = append(all, node.Containers...)
	}
	return all
}

// GetRemoteProxies aggregates proxies from all registered worker nodes.
func (r *ClusterRegistry) GetRemoteProxies() []*ManagedProxy {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var all []*ManagedProxy
	for _, node := range r.nodes {
		all = append(all, node.Proxies...)
	}
	return all
}

// GetRemoteStreams aggregates streams from all registered worker nodes.
func (r *ClusterRegistry) GetRemoteStreams() []*ManagedStream {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var all []*ManagedStream
	for _, node := range r.nodes {
		all = append(all, node.Streams...)
	}
	return all
}

// RegisterNodeReport stores telemetry report and merges logs into live SSE stream.
func (s *Syncer) RegisterNodeReport(report NodeReport) error {
	if strings.TrimSpace(report.NodeID) == "" {
		return fmt.Errorf("node_id must not be empty")
	}

	if s.clusterRegistry == nil {
		s.clusterRegistry = NewClusterRegistry()
	}

	s.clusterRegistry.RegisterOrUpdate(report)

	// Ingest worker's recent events into controller log history & live SSE subscribers
	for _, ev := range report.RecentEvents {
		if ev.NodeID == "" {
			ev.NodeID = report.NodeID
		}
		s.EmitExternalLog(ev)
	}

	s.EmitLog(LevelInfo, "cluster",
		fmt.Sprintf("Received telemetry report from worker node '%s' (%d containers, %d proxies)",
			report.NodeID, len(report.Containers), len(report.Proxies)),
		fmt.Sprintf("Worker IP: %s", report.NodeIP))

	return nil
}

// GetClusterNodes returns all nodes participating in this cluster.
func (s *Syncer) GetClusterNodes() []ClusterNodeInfo {
	overview := s.GetStatusOverview(context.Background())

	s.mu.RLock()
	localProxies := make([]*ManagedProxy, 0, len(s.trackedProxies))
	for _, p := range s.trackedProxies {
		localProxies = append(localProxies, p)
	}
	localStreams := make([]*ManagedStream, 0, len(s.trackedStreams))
	for _, st := range s.trackedStreams {
		localStreams = append(localStreams, st)
	}
	s.mu.RUnlock()

	localNode := ClusterNodeInfo{
		NodeID:          s.cfg.HostID,
		NodeIP:          s.cfg.HostIP,
		IsController:    true,
		Status:          "online",
		LastHeartbeat:   time.Now(),
		UptimeSeconds:   overview.Uptime,
		DockerConnected: overview.DockerConnected,
		DockerVersion:   overview.DockerVersion,
		NPMConnected:    overview.NPMConnected,
		PVEConnected:    overview.PVEConnected,
		LXDConnected:    overview.LXDConnected,
		ContainerCount:  overview.RunningContainers + overview.PVERunningContainers + overview.LXDRunningContainers,
		ProxyCount:      len(localProxies),
		StreamCount:     len(localStreams),
		Overview:        overview,
	}

	if s.clusterRegistry == nil {
		return []ClusterNodeInfo{localNode}
	}

	return s.clusterRegistry.GetNodes(localNode)
}

// triggerWorkerPush queues an immediate telemetry push to the main node.
func (s *Syncer) triggerWorkerPush() {
	if s.cfg.MainNodeURL == "" || s.workerPushTrigger == nil {
		return
	}
	select {
	case s.workerPushTrigger <- struct{}{}:
	default:
	}
}

// startWorkerReporter runs periodic heartbeat and event-driven telemetry pushes to the main node.
func (s *Syncer) startWorkerReporter(ctx context.Context) {
	interval := s.cfg.PushInterval
	if interval < 3*time.Second {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[info] [cluster] Remote Worker Reporter active. Pushing telemetry to %s every %s", s.cfg.MainNodeURL, interval)

	// Send initial report shortly after startup
	select {
	case <-time.After(2 * time.Second):
		s.sendWorkerReport(ctx)
	case <-ctx.Done():
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendWorkerReport(ctx)
		case <-s.workerPushTrigger:
			// Small debounce to let reconciliations settle
			select {
			case <-time.After(500 * time.Millisecond):
				s.sendWorkerReport(ctx)
			case <-ctx.Done():
				return
			}
		}
	}
}

// sendWorkerReport gathers local state and POSTs it to the configured MainNodeURL.
func (s *Syncer) sendWorkerReport(ctx context.Context) {
	if s.cfg.MainNodeURL == "" {
		return
	}

	targetURL := fmt.Sprintf("%s/api/cluster/report", strings.TrimRight(s.cfg.MainNodeURL, "/"))

	overview := s.GetStatusOverview(ctx)
	containers, _ := s.getLocalContainersView(ctx)

	s.mu.RLock()
	localProxies := make([]*ManagedProxy, 0, len(s.trackedProxies))
	for _, p := range s.trackedProxies {
		localProxies = append(localProxies, p)
	}
	localStreams := make([]*ManagedStream, 0, len(s.trackedStreams))
	for _, st := range s.trackedStreams {
		localStreams = append(localStreams, st)
	}
	s.mu.RUnlock()

	// Send recent logs from the last 60 seconds
	since := time.Now().Add(-60 * time.Second)
	recentEvents := s.GetRecentEventsSince(since)

	report := NodeReport{
		NodeID:       s.cfg.HostID,
		NodeIP:       s.cfg.HostIP,
		ReportedAt:   time.Now(),
		Status:       overview,
		Containers:   containers,
		Proxies:      localProxies,
		Streams:      localStreams,
		RecentEvents: recentEvents,
	}

	payload, err := json.Marshal(report)
	if err != nil {
		s.EmitLog(LevelError, "cluster", "Failed to encode worker telemetry report", err.Error())
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, targetURL, bytes.NewReader(payload))
	if err != nil {
		s.EmitLog(LevelError, "cluster", fmt.Sprintf("Failed to initialize push request to main node: %v", err), targetURL)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if s.cfg.ClusterToken != "" {
		req.Header.Set("X-Cluster-Token", s.cfg.ClusterToken)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.EmitLog(LevelWarn, "cluster", fmt.Sprintf("Failed to reach main node at %s: %v", targetURL, err), "")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		s.EmitLog(LevelWarn, "cluster", fmt.Sprintf("Main node rejected telemetry push (HTTP %d)", resp.StatusCode), string(body))
		return
	}
}
