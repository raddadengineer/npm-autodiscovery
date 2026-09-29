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

	targetPVEKey := r.resolveNodeKeyLocked(report.NodeID)
	pveConnected := report.Status.PVEConnected
	if cfgs, ok := r.pveConfigs[targetPVEKey]; ok {
		if len(cfgs) == 0 {
			pveEnabled = false
			pveConnected = false
			pveURL = ""
			pveNode = ""
		} else {
			pveConfigs = make([]PVEConfig, len(cfgs))
			pveEnabled = false
			for i, c := range cfgs {
				cp := *c
				cp.HasSecret = cp.TokenSecret != ""
				pveConfigs[i] = cp
				if c.Enabled {
					pveEnabled = true
				}
			}
			if !pveEnabled {
				pveConnected = false
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
	} else if !pveEnabled {
		pveConnected = false
	}

	overview := report.Status
	if !pveEnabled {
		overview.PVEEnabled = false
		overview.PVEConnected = false
		overview.PVEURL = ""
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
		PVEConnected:          pveConnected,
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
		Overview:              overview,
		Containers:            taggedContainers,
		Proxies:               taggedProxies,
		Streams:               taggedStreams,
	}

	r.nodes[report.NodeID] = info
}

// resolveNodeKeyLocked finds the normalized key for a node in pveConfigs.
func (r *ClusterRegistry) resolveNodeKeyLocked(nodeID string) string {
	cleanID := strings.TrimSpace(nodeID)
	if cleanID == "" {
		cleanID = "controller-main"
	}

	// 1. Exact match in pveConfigs
	if _, ok := r.pveConfigs[cleanID]; ok {
		return cleanID
	}

	// 2. Case-insensitive match in pveConfigs
	for k := range r.pveConfigs {
		if strings.EqualFold(k, cleanID) {
			return k
		}
	}

	// 3. Controller / local aliases
	isLocalAlias := strings.EqualFold(cleanID, "local") || strings.EqualFold(cleanID, "controller-main")
	if isLocalAlias {
		for _, alias := range []string{"controller-main", "local", ""} {
			if _, ok := r.pveConfigs[alias]; ok {
				return alias
			}
		}
	}

	// 4. Check if cleanID matches any registered node in r.nodes (case-insensitive)
	for k := range r.nodes {
		if strings.EqualFold(k, cleanID) {
			if _, ok := r.pveConfigs[k]; ok {
				return k
			}
			for pveKey := range r.pveConfigs {
				if strings.EqualFold(pveKey, k) {
					return pveKey
				}
			}
			return k
		}
	}

	// 5. Single-node fallback if only one node configuration exists in pveConfigs
	if len(r.pveConfigs) == 1 {
		for k := range r.pveConfigs {
			return k
		}
	}

	return cleanID
}

// SetNodePVEConfig stores or updates Proxmox configuration for a cluster node.
func (r *ClusterRegistry) SetNodePVEConfig(nodeID string, cfg PVEConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	targetKey := r.resolveNodeKeyLocked(nodeID)
	if cfg.ID == "" {
		if len(r.pveConfigs[targetKey]) > 0 {
			cfg.ID = r.pveConfigs[targetKey][0].ID
			if cfg.ID == "" {
				cfg.ID = "default"
			}
		} else {
			cfg.ID = "default"
		}
	}

	existingList := r.pveConfigs[targetKey]
	var found *PVEConfig
	for _, c := range existingList {
		if strings.EqualFold(c.ID, cfg.ID) || ((cfg.ID == "default" || cfg.ID == "") && (c.ID == "default" || c.ID == "" || len(existingList) == 1)) {
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
		r.pveConfigs[targetKey] = append(existingList, &cp)
	}

	r.updateNodeInfoPVELocked(targetKey)
	if !strings.EqualFold(targetKey, nodeID) {
		r.updateNodeInfoPVELocked(nodeID)
	}
	r.savePersistedPVEConfigsLocked()
}

// DeleteNodePVEConfig removes a specific Proxmox configuration from a cluster node.
func (r *ClusterRegistry) DeleteNodePVEConfig(nodeID string, configID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	targetKey := r.resolveNodeKeyLocked(nodeID)

	existingList := r.pveConfigs[targetKey]
	if len(existingList) > 0 {
		newList := make([]*PVEConfig, 0, len(existingList))
		deleted := false
		for _, c := range existingList {
			match := false
			if strings.EqualFold(c.ID, configID) {
				match = true
			} else if (configID == "default" || configID == "") && (c.ID == "" || c.ID == "default" || len(existingList) == 1) {
				match = true
			}
			if match {
				deleted = true
				continue
			}
			newList = append(newList, c)
		}

		if deleted {
			r.pveConfigs[targetKey] = newList
			r.updateNodeInfoPVELocked(targetKey)
			if !strings.EqualFold(targetKey, nodeID) {
				r.updateNodeInfoPVELocked(nodeID)
			}
			r.savePersistedPVEConfigsLocked()
			return true
		}
	}

	// If no existingList in pveConfigs, check if node is in r.nodes and has PVE enabled / reporting
	var matchingNode *ClusterNodeInfo
	for k, n := range r.nodes {
		if strings.EqualFold(k, nodeID) || strings.EqualFold(k, targetKey) {
			matchingNode = n
			break
		}
	}

	if matchingNode != nil && (matchingNode.PVEEnabled || matchingNode.PVEURL != "" || matchingNode.Overview.PVEURL != "" || matchingNode.Overview.PVEEnabled || configID == "default" || configID == "") {
		matchingNode.PVEEnabled = false
		matchingNode.PVEConnected = false
		matchingNode.PVEURL = ""
		matchingNode.PVENode = ""
		matchingNode.PVETokenID = ""
		matchingNode.PVEHasSecret = false
		matchingNode.PVEVerifySSL = false
		matchingNode.PVEPreferredInterface = ""
		matchingNode.PVEAllowedSubnets = ""
		matchingNode.PVEConfigs = nil
		matchingNode.PVEEndpointCount = 0
		matchingNode.Overview.PVEEnabled = false
		matchingNode.Overview.PVEURL = ""
		matchingNode.Overview.PVEConnected = false

		r.pveConfigs[targetKey] = []*PVEConfig{}
		if !strings.EqualFold(targetKey, nodeID) {
			r.pveConfigs[nodeID] = []*PVEConfig{}
		}
		r.savePersistedPVEConfigsLocked()
		return true
	}

	// If configID is "default" or empty and list is empty, treat as idempotent success
	if (configID == "default" || configID == "") && len(existingList) == 0 {
		r.pveConfigs[targetKey] = []*PVEConfig{}
		r.savePersistedPVEConfigsLocked()
		return true
	}

	return false
}

// SetNodePVEConfigs replaces all Proxmox configurations for a cluster node.
func (r *ClusterRegistry) SetNodePVEConfigs(nodeID string, cfgs []PVEConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	targetKey := r.resolveNodeKeyLocked(nodeID)

	existingMap := make(map[string]*PVEConfig)
	var firstExistingSecret string
	for _, c := range r.pveConfigs[targetKey] {
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

	r.pveConfigs[targetKey] = newList
	r.updateNodeInfoPVELocked(targetKey)
	if !strings.EqualFold(targetKey, nodeID) {
		r.updateNodeInfoPVELocked(nodeID)
	}
	r.savePersistedPVEConfigsLocked()
}

func (r *ClusterRegistry) updateNodeInfoPVELocked(nodeID string) {
	targetKey := r.resolveNodeKeyLocked(nodeID)
	cfgs := r.pveConfigs[targetKey]

	for k, node := range r.nodes {
		if strings.EqualFold(k, nodeID) || strings.EqualFold(k, targetKey) {
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
					node.Overview.PVEConnected = false
					node.Overview.PVEEnabled = false
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
				node.PVEEnabled = false
				node.PVEConfigs = nil
				node.PVEEndpointCount = 0
				node.Overview.PVEEnabled = false
				node.Overview.PVEURL = ""
				node.Overview.PVEConnected = false
			}
		}
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
// single-node fallback if only one node configuration is persisted, and telemetry fallback.
func (r *ClusterRegistry) GetNodePVEConfigs(nodeID string) []PVEConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	targetKey := r.resolveNodeKeyLocked(nodeID)

	if cfgs, ok := r.pveConfigs[targetKey]; ok {
		if len(cfgs) == 0 {
			return nil
		}
		return copyPVEConfigs(cfgs)
	}

	// Fallback to checking node info in r.nodes if it has active telemetry
	for k, node := range r.nodes {
		if strings.EqualFold(k, nodeID) || strings.EqualFold(k, targetKey) {
			if len(node.PVEConfigs) > 0 {
				res := make([]PVEConfig, len(node.PVEConfigs))
				for i, c := range node.PVEConfigs {
					cp := c
					cp.HasSecret = cp.TokenSecret != ""
					res[i] = cp
				}
				return res
			}
			if node.PVEURL != "" && (node.PVEEnabled || (node.Overview.PVEURL != "" && node.Overview.PVEEnabled)) {
				return []PVEConfig{{
					ID:                 "default",
					Name:               "Primary Proxmox",
					Enabled:            node.PVEEnabled || node.Overview.PVEEnabled,
					URL:                node.PVEURL,
					Node:               node.PVENode,
					TokenID:            node.PVETokenID,
					TokenSecret:        "",
					VerifySSL:          node.PVEVerifySSL,
					PreferredInterface: node.PVEPreferredInterface,
					AllowedSubnets:     node.PVEAllowedSubnets,
					HasSecret:          node.PVEHasSecret,
				}}
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
		if strings.EqualFold(c.ID, configID) || ((configID == "default" || configID == "") && (c.ID == "default" || c.ID == "" || len(cfgs) == 1)) {
			cp := c
			return &cp
		}
	}
	return nil
}

// HasExplicitEmptyPVEConfigs returns true if the node was explicitly configured with 0 endpoints (deleted/disabled).
func (r *ClusterRegistry) HasExplicitEmptyPVEConfigs(nodeID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	targetKey := r.resolveNodeKeyLocked(nodeID)
	cfgs, ok := r.pveConfigs[targetKey]
	return ok && len(cfgs) == 0
}

// GetAllStoredNodeIDs returns all node IDs that have stored Proxmox configurations.
func (r *ClusterRegistry) GetAllStoredNodeIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.pveConfigs))
	for k, list := range r.pveConfigs {
		if len(list) > 0 {
			ids = append(ids, k)
		}
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
		targetKey := r.resolveNodeKeyLocked(cp.NodeID)
		if cfgs, ok := r.pveConfigs[targetKey]; ok {
			if len(cfgs) == 0 {
				cp.PVEConfigs = nil
				cp.PVEEndpointCount = 0
				cp.PVEEnabled = false
				cp.PVEConnected = false
				cp.PVEURL = ""
				cp.PVENode = ""
				cp.PVETokenID = ""
				cp.PVEHasSecret = false
				cp.PVEVerifySSL = false
				cp.PVEPreferredInterface = ""
				cp.PVEAllowedSubnets = ""
				cp.Overview.PVEEnabled = false
				cp.Overview.PVEURL = ""
				cp.Overview.PVEConnected = false
			} else {
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
				if !anyEnabled {
					cp.PVEConnected = false
					cp.Overview.PVEConnected = false
					cp.Overview.PVEEnabled = false
				}
			}
		}
		if !cp.PVEEnabled {
			cp.PVEConnected = false
			cp.Overview.PVEConnected = false
			cp.Overview.PVEEnabled = false
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
		if pushResp.PVEConfigs != nil {
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
