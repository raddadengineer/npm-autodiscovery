package syncer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

// PVEConfig holds configuration parameters to connect to Proxmox VE.
type PVEConfig struct {
	ID                 string `json:"id,omitempty"`
	Name               string `json:"name,omitempty"`
	Enabled            bool   `json:"enabled"`
	URL                string `json:"url"`
	TokenID            string `json:"token_id"`
	TokenSecret        string `json:"token_secret,omitempty"`
	Node               string `json:"node,omitempty"`
	VerifySSL          bool   `json:"verify_ssl"`
	PreferredInterface string `json:"preferred_interface,omitempty"`
	AllowedSubnets     string `json:"allowed_subnets,omitempty"`
	HasSecret          bool   `json:"has_secret,omitempty"`
}

// ClusterNodeInfo contains aggregated status of a cluster node (controller or remote worker).
type ClusterNodeInfo struct {
	NodeID                string                `json:"node_id"`
	NodeIP                string                `json:"node_ip"`
	IsController          bool                  `json:"is_controller"`
	Status                string                `json:"status"` // "online", "offline"
	LastHeartbeat         time.Time             `json:"last_heartbeat"`
	UptimeSeconds         int64                 `json:"uptime_seconds"`
	DockerConnected       bool                  `json:"docker_connected"`
	DockerVersion         string                `json:"docker_version"`
	NPMConnected          bool                  `json:"npm_connected"`
	PVEEnabled            bool                  `json:"pve_enabled"`
	PVEConnected          bool                  `json:"pve_connected"`
	PVEURL                string                `json:"pve_url,omitempty"`
	PVENode               string                `json:"pve_node,omitempty"`
	PVEVersion            string                `json:"pve_version,omitempty"`
	PVETokenID            string                `json:"pve_token_id,omitempty"`
	PVEHasSecret          bool                  `json:"pve_has_secret"`
	PVEVerifySSL          bool                  `json:"pve_verify_ssl"`
	PVEPreferredInterface string                `json:"pve_preferred_interface,omitempty"`
	PVEAllowedSubnets     string                `json:"pve_allowed_subnets,omitempty"`
	PVEConfigs            []PVEConfig           `json:"pve_configs,omitempty"`
	PVEEndpointCount      int                   `json:"pve_endpoint_count,omitempty"`
	LXDConnected          bool                  `json:"lxd_connected"`
	ContainerCount        int                   `json:"container_count"`
	ProxyCount            int                   `json:"proxy_count"`
	StreamCount           int                   `json:"stream_count"`
	Overview              StatusOverview        `json:"overview"`
	Containers            []ContainerStatusView `json:"containers,omitempty"`
	Proxies               []*ManagedProxy       `json:"proxies,omitempty"`
	Streams               []*ManagedStream      `json:"streams,omitempty"`
}

// ClusterRegistry stores and coordinates remote worker telemetry on the controller node.
type ClusterRegistry struct {
	mu         sync.RWMutex
	nodes      map[string]*ClusterNodeInfo // Key: NodeID
	pveConfigs map[string][]*PVEConfig     // Key: NodeID -> list of PVEConfigs
	configPath string
}

// NewClusterRegistry creates an empty registry for tracking cluster nodes and loads persisted configs.
func NewClusterRegistry() *ClusterRegistry {
	reg := &ClusterRegistry{
		nodes:      make(map[string]*ClusterNodeInfo),
		pveConfigs: make(map[string][]*PVEConfig),
		configPath: resolveClusterConfigPath(),
	}
	reg.loadPersistedPVEConfigs()
	return reg
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

	pveEnabled := report.Status.PVEEnabled
	pveURL := report.Status.PVEURL
	pveNode := report.Status.PVENode
	pveTokenID := ""
	pveHasSecret := false
	pveVerifySSL := false
	pvePrefIface := "eth0"
	pveSubnets := ""
	var pveConfigs []PVEConfig

	if cfgs, ok := r.pveConfigs[report.NodeID]; ok && len(cfgs) > 0 {
		pveConfigs = make([]PVEConfig, len(cfgs))
		for i, c := range cfgs {
			cp := *c
			cp.HasSecret = cp.TokenSecret != ""
			pveConfigs[i] = cp
			if c.Enabled {
				pveEnabled = true
			}
		}
		primary := cfgs[0]
		for _, c := range cfgs {
			if c.Enabled {
				primary = c
				break
			}
		}
		if primary.URL != "" {
			pveURL = primary.URL
		}
		if primary.Node != "" {
			pveNode = primary.Node
		}
		pveTokenID = primary.TokenID
		pveHasSecret = primary.HasSecret
		pveVerifySSL = primary.VerifySSL
		pvePrefIface = primary.PreferredInterface
		pveSubnets = primary.AllowedSubnets
	}

	info := &ClusterNodeInfo{
		NodeID:                report.NodeID,
		NodeIP:                report.NodeIP,
		IsController:          false,
		Status:                "online",
		LastHeartbeat:         time.Now(),
		UptimeSeconds:         report.Status.Uptime,
		DockerConnected:       report.Status.DockerConnected,
		DockerVersion:         report.Status.DockerVersion,
		NPMConnected:          report.Status.NPMConnected,
		PVEEnabled:            pveEnabled,
		PVEConnected:          report.Status.PVEConnected,
		PVEURL:                pveURL,
		PVENode:               pveNode,
		PVEVersion:            report.Status.PVEVersion,
		PVETokenID:            pveTokenID,
		PVEHasSecret:          pveHasSecret,
		PVEVerifySSL:          pveVerifySSL,
		PVEPreferredInterface: pvePrefIface,
		PVEAllowedSubnets:     pveSubnets,
		PVEConfigs:            pveConfigs,
		PVEEndpointCount:      len(pveConfigs),
		LXDConnected:          report.Status.LXDConnected,
		ContainerCount:        len(taggedContainers),
		ProxyCount:            len(taggedProxies),
		StreamCount:           len(taggedStreams),
		Overview:              report.Status,
		Containers:            taggedContainers,
		Proxies:               taggedProxies,
		Streams:               taggedStreams,
	}

	r.nodes[report.NodeID] = info
}

// SetNodePVEConfig stores or updates Proxmox configuration for a cluster node.
func (r *ClusterRegistry) SetNodePVEConfig(nodeID string, cfg PVEConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if nodeID == "" {
		nodeID = "controller-main"
	}

	if cfg.ID == "" {
		if len(r.pveConfigs[nodeID]) > 0 {
			cfg.ID = r.pveConfigs[nodeID][0].ID
			if cfg.ID == "" {
				cfg.ID = "default"
			}
		} else {
			cfg.ID = "default"
		}
	}

	existingList := r.pveConfigs[nodeID]
	var found *PVEConfig
	for _, c := range existingList {
		if c.ID == cfg.ID || (cfg.ID == "default" && (c.ID == "default" || c.ID == "")) {
			found = c
			break
		}
	}

	// If secret was not provided in update, keep existing secret
	if cfg.TokenSecret == "" {
		if found != nil && found.TokenSecret != "" {
			cfg.TokenSecret = found.TokenSecret
		} else if len(existingList) == 1 && existingList[0].TokenSecret != "" {
			cfg.TokenSecret = existingList[0].TokenSecret
		}
	}
	cfg.HasSecret = cfg.TokenSecret != ""

	cp := cfg
	if found != nil {
		*found = cp
	} else {
		r.pveConfigs[nodeID] = append(existingList, &cp)
	}

	r.updateNodeInfoPVELocked(nodeID)
	r.savePersistedPVEConfigsLocked()
}

// DeleteNodePVEConfig removes a specific Proxmox configuration from a cluster node.
func (r *ClusterRegistry) DeleteNodePVEConfig(nodeID string, configID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if nodeID == "" {
		nodeID = "controller-main"
	}

	existingList, ok := r.pveConfigs[nodeID]
	if !ok || len(existingList) == 0 {
		return false
	}

	newList := make([]*PVEConfig, 0, len(existingList))
	deleted := false
	for _, c := range existingList {
		if c.ID == configID || (configID == "default" && c.ID == "") {
			deleted = true
			continue
		}
		newList = append(newList, c)
	}

	if deleted {
		r.pveConfigs[nodeID] = newList
		r.updateNodeInfoPVELocked(nodeID)
		r.savePersistedPVEConfigsLocked()
	}
	return deleted
}

// SetNodePVEConfigs replaces all Proxmox configurations for a cluster node.
func (r *ClusterRegistry) SetNodePVEConfigs(nodeID string, cfgs []PVEConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if nodeID == "" {
		nodeID = "controller-main"
	}

	existingMap := make(map[string]*PVEConfig)
	var firstExistingSecret string
	for _, c := range r.pveConfigs[nodeID] {
		if c.ID != "" {
			existingMap[c.ID] = c
		}
		if firstExistingSecret == "" && c.TokenSecret != "" {
			firstExistingSecret = c.TokenSecret
		}
	}

	newList := make([]*PVEConfig, 0, len(cfgs))
	for i, c := range cfgs {
		cp := c
		if cp.ID == "" {
			cp.ID = fmt.Sprintf("pve-%d-%d", time.Now().UnixNano(), i+1)
		}
		if cp.TokenSecret == "" {
			if existing, ok := existingMap[cp.ID]; ok && existing.TokenSecret != "" {
				cp.TokenSecret = existing.TokenSecret
			} else if len(cfgs) == 1 && firstExistingSecret != "" {
				cp.TokenSecret = firstExistingSecret
			}
		}
		cp.HasSecret = cp.TokenSecret != ""
		newList = append(newList, &cp)
	}

	r.pveConfigs[nodeID] = newList
	r.updateNodeInfoPVELocked(nodeID)
	r.savePersistedPVEConfigsLocked()
}

func (r *ClusterRegistry) updateNodeInfoPVELocked(nodeID string) {
	node, ok := r.nodes[nodeID]
	if !ok {
		return
	}

	cfgs := r.pveConfigs[nodeID]
	node.PVEConfigs = make([]PVEConfig, len(cfgs))
	anyEnabled := false
	for i, c := range cfgs {
		cp := *c
		cp.HasSecret = cp.TokenSecret != ""
		node.PVEConfigs[i] = cp
		if c.Enabled {
			anyEnabled = true
		}
	}
	node.PVEEnabled = anyEnabled
	node.PVEEndpointCount = len(cfgs)

	if len(cfgs) > 0 {
		primary := cfgs[0]
		for _, c := range cfgs {
			if c.Enabled {
				primary = c
				break
			}
		}
		node.PVEURL = primary.URL
		node.PVENode = primary.Node
		node.PVETokenID = primary.TokenID
		node.PVEHasSecret = primary.HasSecret
		node.PVEVerifySSL = primary.VerifySSL
		node.PVEPreferredInterface = primary.PreferredInterface
		node.PVEAllowedSubnets = primary.AllowedSubnets
		if !anyEnabled {
			node.PVEConnected = false
		}
	} else {
		node.PVEURL = ""
		node.PVENode = ""
		node.PVETokenID = ""
		node.PVEHasSecret = false
		node.PVEVerifySSL = false
		node.PVEPreferredInterface = ""
		node.PVEAllowedSubnets = ""
		node.PVEConnected = false
	}
}

func copyPVEConfigs(cfgs []*PVEConfig) []PVEConfig {
	result := make([]PVEConfig, len(cfgs))
	for i, c := range cfgs {
		cp := *c
		cp.HasSecret = cp.TokenSecret != ""
		result[i] = cp
	}
	return result
}

// GetNodePVEConfigs retrieves all configured Proxmox settings for a node.
// It supports exact match, case-insensitive match, common aliases (local, controller-main),
// and single-node fallback if only one node configuration is persisted.
func (r *ClusterRegistry) GetNodePVEConfigs(nodeID string) []PVEConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. Exact match
	if cfgs, ok := r.pveConfigs[nodeID]; ok && len(cfgs) > 0 {
		return copyPVEConfigs(cfgs)
	}

	// 2. Case-insensitive match
	for k, cfgs := range r.pveConfigs {
		if strings.EqualFold(k, nodeID) && len(cfgs) > 0 {
			return copyPVEConfigs(cfgs)
		}
	}

	// 3. Common controller / local aliases
	isLocalAlias := nodeID == "" || strings.EqualFold(nodeID, "local") || strings.EqualFold(nodeID, "controller-main")
	if isLocalAlias {
		for _, alias := range []string{"controller-main", "local", ""} {
			if cfgs, ok := r.pveConfigs[alias]; ok && len(cfgs) > 0 {
				return copyPVEConfigs(cfgs)
			}
		}
	}

	// 4. Single-node fallback: If there is exactly one configured node in the registry,
	// return it for local queries or general fallback so upgrades never clear configs.
	if len(r.pveConfigs) == 1 {
		for _, cfgs := range r.pveConfigs {
			if len(cfgs) > 0 {
				return copyPVEConfigs(cfgs)
			}
		}
	}

	return nil
}

// GetNodePVEConfig retrieves the primary configured Proxmox settings for a node.
func (r *ClusterRegistry) GetNodePVEConfig(nodeID string) *PVEConfig {
	cfgs := r.GetNodePVEConfigs(nodeID)
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

// GetNodePVEConfigByID retrieves a specific Proxmox config by ID.
func (r *ClusterRegistry) GetNodePVEConfigByID(nodeID string, configID string) *PVEConfig {
	cfgs := r.GetNodePVEConfigs(nodeID)
	for _, c := range cfgs {
		if c.ID == configID || (configID == "default" && (c.ID == "default" || c.ID == "")) {
			cp := c
			return &cp
		}
	}
	return nil
}

// GetAllStoredNodeIDs returns all node IDs that have stored Proxmox configurations.
func (r *ClusterRegistry) GetAllStoredNodeIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.pveConfigs))
	for k := range r.pveConfigs {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	return ids
}

// GetAllNodePVEConfigs returns all stored node PVE configs (primary per node).
func (r *ClusterRegistry) GetAllNodePVEConfigs() map[string]PVEConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]PVEConfig, len(r.pveConfigs))
	for k, cfgs := range r.pveConfigs {
		if len(cfgs) > 0 {
			primary := cfgs[0]
			for _, c := range cfgs {
				if c.Enabled {
					primary = c
					break
				}
			}
			cp := *primary
			cp.HasSecret = cp.TokenSecret != ""
			result[k] = cp
		}
	}
	return result
}

func (r *ClusterRegistry) savePersistedPVEConfigsLocked() {
	if r.configPath == "" {
		return
	}
	data, err := json.MarshalIndent(r.pveConfigs, "", "  ")
	if err != nil {
		log.Printf("[cluster] ERROR: Failed to serialize Proxmox configurations: %v", err)
		return
	}

	dir := filepath.Dir(r.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[cluster] ERROR: Failed to create persistence directory %s: %v", dir, err)
	}

	// Atomic write using temporary file to prevent corruption on abrupt restart
	tmpPath := fmt.Sprintf("%s.tmp.%d", r.configPath, time.Now().UnixNano())
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		log.Printf("[cluster] ERROR: Failed to write temporary config %s: %v", tmpPath, err)
		_ = os.WriteFile(r.configPath, data, 0600)
	} else {
		if err := os.Rename(tmpPath, r.configPath); err != nil {
			log.Printf("[cluster] ERROR: Failed to rename %s to %s: %v", tmpPath, r.configPath, err)
			_ = os.WriteFile(r.configPath, data, 0600)
			_ = os.Remove(tmpPath)
		}
	}

	log.Printf("[cluster] Persisted Proxmox configurations for %d node(s) to %s", len(r.pveConfigs), r.configPath)
}

func (r *ClusterRegistry) loadPersistedPVEConfigs() {
	var loadedData []byte
	var loadedFrom string

	// 1. Check primary config path first
	if r.configPath != "" {
		if data, err := os.ReadFile(r.configPath); err == nil && len(data) > 0 {
			loadedData = data
			loadedFrom = r.configPath
		}
	}

	// 2. Only if primary doesn't exist and DATA_DIR was not explicitly set, search legacy fallback paths
	if len(loadedData) == 0 && os.Getenv("DATA_DIR") == "" {
		candidatePaths := []string{
			"/data/cluster_pve_configs.json",
			"data/cluster_pve_configs.json",
			".cluster_pve_configs.json",
			"/app/.cluster_pve_configs.json",
			"/app/data/cluster_pve_configs.json",
		}
		for _, p := range candidatePaths {
			if p == r.configPath {
				continue
			}
			if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
				loadedData = data
				loadedFrom = p
				break
			}
		}
	}

	if len(loadedData) == 0 {
		return
	}

	// 1. Try modern multi-config format map[string][]*PVEConfig
	var multiLoaded map[string][]*PVEConfig
	if err := json.Unmarshal(loadedData, &multiLoaded); err == nil && len(multiLoaded) > 0 {
		for k, list := range multiLoaded {
			var valid []*PVEConfig
			for _, v := range list {
				if v != nil {
					v.HasSecret = v.TokenSecret != ""
					if v.ID == "" {
						v.ID = fmt.Sprintf("pve-%d", time.Now().UnixNano())
					}
					valid = append(valid, v)
				}
			}
			r.pveConfigs[k] = valid
		}
		log.Printf("[cluster] Loaded Proxmox configurations for %d node(s) from %s", len(r.pveConfigs), loadedFrom)
		// If loaded from a fallback path, migrate into primary config path immediately
		if r.configPath != "" && loadedFrom != r.configPath {
			r.savePersistedPVEConfigsLocked()
		}
		return
	}

	// 2. Fallback to legacy single-config format map[string]*PVEConfig
	var singleLoaded map[string]*PVEConfig
	if err := json.Unmarshal(loadedData, &singleLoaded); err == nil {
		for k, v := range singleLoaded {
			if v != nil {
				v.HasSecret = v.TokenSecret != ""
				if v.ID == "" {
					v.ID = "default"
				}
				r.pveConfigs[k] = []*PVEConfig{v}
			}
		}
		log.Printf("[cluster] Migrated legacy Proxmox configurations for %d node(s) from %s", len(r.pveConfigs), loadedFrom)
		if r.configPath != "" {
			r.savePersistedPVEConfigsLocked()
		}
	}
}

func resolveClusterConfigPath() string {
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		_ = os.MkdirAll(dir, 0755)
		return filepath.Join(dir, "cluster_pve_configs.json")
	}
	if fi, err := os.Stat("/data"); err == nil && fi.IsDir() {
		return "/data/cluster_pve_configs.json"
	}
	if fi, err := os.Stat("data"); err == nil && fi.IsDir() {
		return "data/cluster_pve_configs.json"
	}
	if _, err := os.Stat("/app"); err == nil {
		_ = os.MkdirAll("/data", 0755)
		if fi, err := os.Stat("/data"); err == nil && fi.IsDir() {
			return "/data/cluster_pve_configs.json"
		}
	}
	_ = os.MkdirAll("data", 0755)
	return "data/cluster_pve_configs.json"
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
		if cfgs, ok := r.pveConfigs[cp.NodeID]; ok && len(cfgs) > 0 {
			cp.PVEConfigs = make([]PVEConfig, len(cfgs))
			anyEnabled := false
			for i, c := range cfgs {
				cfgCopy := *c
				cfgCopy.HasSecret = cfgCopy.TokenSecret != ""
				cp.PVEConfigs[i] = cfgCopy
				if c.Enabled {
					anyEnabled = true
				}
			}
			cp.PVEEnabled = anyEnabled
			cp.PVEEndpointCount = len(cfgs)

			primary := cfgs[0]
			for _, c := range cfgs {
				if c.Enabled {
					primary = c
					break
				}
			}
			if primary.URL != "" {
				cp.PVEURL = primary.URL
			}
			if primary.Node != "" {
				cp.PVENode = primary.Node
			}
			cp.PVETokenID = primary.TokenID
			cp.PVEHasSecret = primary.HasSecret
			cp.PVEVerifySSL = primary.VerifySSL
			cp.PVEPreferredInterface = primary.PreferredInterface
			cp.PVEAllowedSubnets = primary.AllowedSubnets
		}
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

	localNodeID := s.cfg.HostID
	if strings.TrimSpace(localNodeID) == "" {
		localNodeID = "controller-main"
	}
	localNodeIP := s.cfg.HostIP
	if strings.TrimSpace(localNodeIP) == "" {
		localNodeIP = "127.0.0.1"
	}

	localCfgs := s.GetNodePVEConfigs(localNodeID)

	localNode := ClusterNodeInfo{
		NodeID:                localNodeID,
		NodeIP:                localNodeIP,
		IsController:          true,
		Status:                "online",
		LastHeartbeat:         time.Now(),
		UptimeSeconds:         overview.Uptime,
		DockerConnected:       overview.DockerConnected,
		DockerVersion:         overview.DockerVersion,
		NPMConnected:          overview.NPMConnected,
		PVEEnabled:            s.cfg.PVEEnabled || overview.PVEEnabled,
		PVEConnected:          overview.PVEConnected,
		PVEURL:                overview.PVEURL,
		PVENode:               overview.PVENode,
		PVEVersion:            overview.PVEVersion,
		PVETokenID:            s.cfg.PVETokenID,
		PVEHasSecret:          s.cfg.PVETokenSecret != "",
		PVEVerifySSL:          s.cfg.PVEVerifySSL,
		PVEPreferredInterface: s.cfg.PVEPreferredInterface,
		PVEAllowedSubnets:     s.cfg.PVEAllowedSubnets,
		PVEConfigs:            localCfgs,
		PVEEndpointCount:      len(localCfgs),
		LXDConnected:          overview.LXDConnected,
		ContainerCount:        overview.RunningContainers + overview.PVERunningContainers + overview.LXDRunningContainers,
		ProxyCount:            len(localProxies),
		StreamCount:           len(localStreams),
		Overview:              overview,
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

	var pushResp struct {
		Status     string      `json:"status"`
		Message    string      `json:"message"`
		PVEConfig  *PVEConfig  `json:"pve_config,omitempty"`
		PVEConfigs []PVEConfig `json:"pve_configs,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pushResp); err == nil {
		if len(pushResp.PVEConfigs) > 0 {
			_ = s.ApplyDynamicPVEConfigs(pushResp.PVEConfigs)
			if s.clusterRegistry != nil {
				s.clusterRegistry.SetNodePVEConfigs(s.cfg.HostID, pushResp.PVEConfigs)
			}
		} else if pushResp.PVEConfig != nil {
			_ = s.ApplyDynamicPVEConfig(*pushResp.PVEConfig)
			if s.clusterRegistry != nil {
				s.clusterRegistry.SetNodePVEConfig(s.cfg.HostID, *pushResp.PVEConfig)
			}
		}
	}
}
