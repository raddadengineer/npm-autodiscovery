package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/raddadengineer/npm-autodiscovery/internal/config"
	"github.com/raddadengineer/npm-autodiscovery/internal/metrics"
	"github.com/raddadengineer/npm-autodiscovery/internal/pve"
	"github.com/raddadengineer/npm-autodiscovery/internal/syncer"
)

// Server handles HTTP API requests and serves the frontend Single Page Application.
type Server struct {
	cfg       *config.Config
	syncer    *syncer.Syncer
	webFS     fs.FS
	httpSrv   *http.Server
}

// NewServer initializes an HTTP server.
func NewServer(cfg *config.Config, s *syncer.Syncer, webFS fs.FS) *Server {
	return &Server{
		cfg:    cfg,
		syncer: s,
		webFS:  webFS,
	}
}

// Start runs the HTTP listener on the configured port.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API Routes
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/proxies", s.handleProxies)
	mux.HandleFunc("/api/streams", s.handleStreams)
	mux.HandleFunc("/api/containers", s.handleContainers)
	mux.HandleFunc("/api/sync", s.handleSync)
	mux.HandleFunc("/api/events/stream", s.handleEventsStream)
	mux.HandleFunc("/api/events", s.handleEventsHistory)
	mux.HandleFunc("/api/config", s.handleConfig)

	// Multi-Node Cluster Control Plane Routes
	mux.HandleFunc("/api/cluster/nodes", s.handleClusterNodes)
	mux.HandleFunc("/api/cluster/report", s.handleClusterReport)
	mux.HandleFunc("/api/cluster/setup-info", s.handleClusterSetupInfo)
	mux.HandleFunc("/api/cluster/nodes/{nodeId}/proxmox", s.handleNodeProxmox)
	mux.HandleFunc("/api/proxmox/config", s.handleLocalProxmoxConfig)
	mux.HandleFunc("/api/proxmox/test", s.handleProxmoxTest)

	// Prometheus Metrics Endpoint (Phase 1)
	mux.HandleFunc("/metrics", s.handleMetrics)

	// Static Web Dashboard files & SPA client routing fallback
	if s.webFS != nil {
		fileServer := http.FileServer(http.FS(s.webFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Don't intercept API routes or Prometheus metrics
			if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/metrics" {
				http.NotFound(w, r)
				return
			}

			// Clean relative path in webFS
			cleanPath := strings.TrimPrefix(r.URL.Path, "/")
			if cleanPath == "" {
				fileServer.ServeHTTP(w, r)
				return
			}

			// If the static asset exists directly (e.g. css/style.css, js/app.js, favicon), serve it
			if f, err := s.webFS.Open(cleanPath); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}

			// Otherwise, fall back to index.html for client-side SPA routes (e.g. /containers, /proxies, /streams, /cluster, /events, /docs)
			indexBytes, err := fs.ReadFile(s.webFS, "index.html")
			if err != nil {
				fileServer.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write(indexBytes)
		})
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("NPM Auto-Discovery Backend API is running."))
		})
	}

	addr := fmt.Sprintf(":%d", s.cfg.Port)
	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      s.withMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // 0 for SSE streaming connections
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("[info] [server] Web Dashboard & API listening on http://0.0.0.0:%d", s.cfg.Port)
	return s.httpSrv.ListenAndServe()
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	if s.httpSrv != nil {
		return s.httpSrv.Shutdown(ctx)
	}
	return nil
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleStatus returns overall status, connection health, and metrics.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	overview := s.syncer.GetStatusOverview(r.Context())
	s.writeJSON(w, http.StatusOK, overview)
}

// handleProxies returns active auto-discovered proxy hosts.
func (s *Server) handleProxies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	proxies := s.syncer.GetTrackedProxies()
	if nodeFilter := strings.TrimSpace(r.URL.Query().Get("node")); nodeFilter != "" && nodeFilter != "all" {
		filtered := make([]*syncer.ManagedProxy, 0)
		for _, p := range proxies {
			if strings.EqualFold(p.HostID, nodeFilter) {
				filtered = append(filtered, p)
			}
		}
		proxies = filtered
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"proxies": proxies,
		"count":   len(proxies),
	})
}

// handleStreams returns active auto-discovered Layer 4 streams.
func (s *Server) handleStreams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	streams := s.syncer.GetTrackedStreams()
	if nodeFilter := strings.TrimSpace(r.URL.Query().Get("node")); nodeFilter != "" && nodeFilter != "all" {
		filtered := make([]*syncer.ManagedStream, 0)
		for _, st := range streams {
			if strings.EqualFold(st.HostID, nodeFilter) {
				filtered = append(filtered, st)
			}
		}
		streams = filtered
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"streams": streams,
		"count":   len(streams),
	})
}

// handleContainers returns running containers with discovery evaluation.
func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	containers, err := s.syncer.GetContainersView(r.Context())
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	if nodeFilter := strings.TrimSpace(r.URL.Query().Get("node")); nodeFilter != "" && nodeFilter != "all" {
		filtered := make([]syncer.ContainerStatusView, 0)
		for _, c := range containers {
			if strings.EqualFold(c.NodeID, nodeFilter) {
				filtered = append(filtered, c)
			}
		}
		containers = filtered
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"containers": containers,
		"count":      len(containers),
	})
}

// handleSync triggers an immediate full scan and reconciliation.
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.syncer.TriggerSync()
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "sync_triggered",
		"message": "Full discovery scan initiated immediately",
	})
}

// handleEventsHistory returns recent log events.
func (s *Server) handleEventsHistory(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	events := s.syncer.GetLogEvents(limit)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"events": events,
		"count":  len(events),
	})
}

// handleEventsStream streams live logs via Server-Sent Events (SSE).
func (s *Server) handleEventsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial ping comment
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	// Send recent logs first so client sees immediate context
	history := s.syncer.GetLogEvents(25)
	for _, ev := range history {
		if data, err := json.Marshal(ev); err == nil {
			fmt.Fprintf(w, "event: log\ndata: %s\n\n", string(data))
		}
	}
	flusher.Flush()

	eventCh, unsubscribe := s.syncer.SubscribeLogs()
	defer unsubscribe()

	keepAliveTicker := time.NewTicker(15 * time.Second)
	defer keepAliveTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case ev, ok := <-eventCh:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err == nil {
				fmt.Fprintf(w, "event: log\ndata: %s\n\n", string(data))
				flusher.Flush()
			}

		case <-keepAliveTicker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// handleConfig returns public configuration details.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, s.cfg)
}

// handleMetrics serves standard Prometheus metrics.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics.Handler().ServeHTTP(w, r)
}

// handleClusterNodes returns all nodes participating in the multi-host cluster.
func (s *Server) handleClusterNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nodes := s.syncer.GetClusterNodes()
	onlineCount := 0
	for _, n := range nodes {
		if n.Status == "online" {
			onlineCount++
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"nodes":        nodes,
		"count":        len(nodes),
		"online_count": onlineCount,
	})
}

// handleClusterReport ingests telemetry pushed from a remote worker node.
func (s *Server) handleClusterReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify pre-shared cluster authentication token if configured
	if s.cfg.ClusterToken != "" {
		token := r.Header.Get("X-Cluster-Token")
		if token == "" {
			authHeader := r.Header.Get("Authorization")
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
		if token != s.cfg.ClusterToken {
			http.Error(w, "Unauthorized: Invalid cluster authentication token", http.StatusUnauthorized)
			return
		}
	}

	var report syncer.NodeReport
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON telemetry payload: %v", err), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(report.NodeID) == "" {
		http.Error(w, "Field 'node_id' is required", http.StatusBadRequest)
		return
	}

	if err := s.syncer.RegisterNodeReport(report); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pveConfigs := s.syncer.GetNodePVEConfigs(report.NodeID)
	resp := map[string]interface{}{
		"status":  "ok",
		"message": "Telemetry report successfully ingested",
		"node_id": report.NodeID,
	}
	if len(pveConfigs) > 0 {
		resp["pve_configs"] = pveConfigs
		resp["pve_config"] = pveConfigs[0]
	} else if s.syncer.HasExplicitEmptyPVEConfigs(report.NodeID) {
		resp["pve_configs"] = []syncer.PVEConfig{}
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleClusterSetupInfo returns configuration parameters needed to provision a new worker node.
func (s *Server) handleClusterSetupInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mainNodeURL := s.cfg.MainNodeURL
	if mainNodeURL == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		host := r.Host
		if host == "" {
			if s.cfg.HostIP != "" {
				host = fmt.Sprintf("%s:%d", s.cfg.HostIP, s.cfg.Port)
			} else {
				host = fmt.Sprintf("127.0.0.1:%d", s.cfg.Port)
			}
		}
		mainNodeURL = fmt.Sprintf("%s://%s", scheme, host)
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"cluster_token":            s.cfg.ClusterToken,
		"cluster_token_configured": s.cfg.ClusterToken != "",
		"main_node_url":            mainNodeURL,
		"controller_id":            s.cfg.HostID,
		"controller_ip":            s.cfg.HostIP,
		"npm_url":                  s.cfg.NPMURL,
		"npm_user":                 s.cfg.NPMUser,
		"default_websocket":        s.cfg.DefaultWebsocket,
		"default_block_exploits":   s.cfg.DefaultBlockExploits,
		"auto_detect_ssl":          s.cfg.AutoDetectSSL,
	})
}

// handleNodeProxmox gets, updates, or deletes Proxmox configurations for a specific cluster node.
func (s *Server) handleNodeProxmox(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("nodeId")
	if nodeID == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 4 {
			nodeID = parts[3]
		}
	}
	if nodeID == "" {
		http.Error(w, "Node ID required in URL path", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfgs := s.syncer.GetNodePVEConfigs(nodeID)
		primary := s.syncer.GetNodePVEConfig(nodeID)
		if primary == nil {
			primary = &syncer.PVEConfig{
				Enabled: false,
			}
		}

		safeCfgs := make([]syncer.PVEConfig, len(cfgs))
		for i, c := range cfgs {
			cp := c
			cp.TokenSecret = "" // mask secret
			safeCfgs[i] = cp
		}

		resp := map[string]interface{}{
			"configs":             safeCfgs,
			"enabled":             primary.Enabled,
			"url":                 primary.URL,
			"token_id":            primary.TokenID,
			"token_secret":        "",
			"node":                primary.Node,
			"verify_ssl":          primary.VerifySSL,
			"preferred_interface": primary.PreferredInterface,
			"allowed_subnets":     primary.AllowedSubnets,
			"has_secret":          primary.HasSecret,
			"id":                  primary.ID,
			"name":                primary.Name,
		}
		s.writeJSON(w, http.StatusOK, resp)

	case http.MethodPost:
		rawBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed reading request body", http.StatusBadRequest)
			return
		}

		var body map[string]interface{}
		if err := json.Unmarshal(rawBytes, &body); err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}

		action, _ := body["action"].(string)
		if strings.EqualFold(action, "delete") {
			configID, _ := body["id"].(string)
			if configID == "" {
				http.Error(w, "Config ID required for deletion", http.StatusBadRequest)
				return
			}
			if err := s.syncer.DeleteNodePVEConfig(nodeID, configID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			s.writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":  "ok",
				"message": fmt.Sprintf("Proxmox VE configuration '%s' deleted for node '%s'", configID, nodeID),
				"node_id": nodeID,
			})
			return
		}

		if _, hasConfigs := body["configs"]; hasConfigs {
			var bulkReq struct {
				Configs []syncer.PVEConfig `json:"configs"`
			}
			if err := json.Unmarshal(rawBytes, &bulkReq); err != nil {
				http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
				return
			}
			if err := s.syncer.SetNodePVEConfigs(nodeID, bulkReq.Configs); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			s.writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":  "ok",
				"message": fmt.Sprintf("Proxmox VE configurations updated for node '%s'", nodeID),
				"node_id": nodeID,
				"count":   len(bulkReq.Configs),
			})
			return
		}

		var pveCfg syncer.PVEConfig
		if err := json.Unmarshal(rawBytes, &pveCfg); err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}

		if err := s.syncer.SetNodePVEConfig(nodeID, pveCfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":    "ok",
			"message":   fmt.Sprintf("Proxmox VE configuration updated for node '%s'", nodeID),
			"node_id":   nodeID,
			"config_id": pveCfg.ID,
			"enabled":   pveCfg.Enabled,
		})

	case http.MethodDelete:
		configID := r.URL.Query().Get("id")
		if configID == "" {
			http.Error(w, "Query parameter 'id' required for deletion", http.StatusBadRequest)
			return
		}
		if err := s.syncer.DeleteNodePVEConfig(nodeID, configID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "ok",
			"message": fmt.Sprintf("Proxmox VE configuration '%s' deleted for node '%s'", configID, nodeID),
			"node_id": nodeID,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLocalProxmoxConfig gets, updates, or deletes Proxmox configurations for the local node.
func (s *Server) handleLocalProxmoxConfig(w http.ResponseWriter, r *http.Request) {
	nodeID := s.cfg.HostID
	if nodeID == "" {
		nodeID = "controller-main"
	}

	switch r.Method {
	case http.MethodGet:
		cfgs := s.syncer.GetNodePVEConfigs(nodeID)
		primary := s.syncer.GetNodePVEConfig(nodeID)
		if primary == nil {
			primary = &syncer.PVEConfig{
				Enabled: false,
			}
		}
		safeCfgs := make([]syncer.PVEConfig, len(cfgs))
		for i, c := range cfgs {
			cp := c
			cp.TokenSecret = ""
			safeCfgs[i] = cp
		}
		resp := map[string]interface{}{
			"configs":             safeCfgs,
			"enabled":             primary.Enabled,
			"url":                 primary.URL,
			"token_id":            primary.TokenID,
			"token_secret":        "",
			"node":                primary.Node,
			"verify_ssl":          primary.VerifySSL,
			"preferred_interface": primary.PreferredInterface,
			"allowed_subnets":     primary.AllowedSubnets,
			"has_secret":          primary.HasSecret,
			"id":                  primary.ID,
			"name":                primary.Name,
		}
		s.writeJSON(w, http.StatusOK, resp)

	case http.MethodPost:
		var pveCfg syncer.PVEConfig
		if err := json.NewDecoder(r.Body).Decode(&pveCfg); err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}

		if err := s.syncer.SetNodePVEConfig(nodeID, pveCfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "ok",
			"message": "Proxmox VE configuration updated for local node",
			"node_id": nodeID,
			"enabled": pveCfg.Enabled,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProxmoxTest tests a Proxmox VE connection using supplied credentials.
func (s *Server) handleProxmoxTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		NodeID             string `json:"node_id"`
		ConfigID           string `json:"config_id"`
		URL                string `json:"url"`
		TokenID            string `json:"token_id"`
		TokenSecret        string `json:"token_secret"`
		Node               string `json:"node"`
		VerifySSL          bool   `json:"verify_ssl"`
		PreferredInterface string `json:"preferred_interface"`
		AllowedSubnets     string `json:"allowed_subnets"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Proxmox VE API URL must not be empty",
		})
		return
	}

	secret := req.TokenSecret
	if secret == "" && req.NodeID != "" {
		if req.ConfigID != "" {
			if existing := s.syncer.GetNodePVEConfigByID(req.NodeID, req.ConfigID); existing != nil && existing.TokenSecret != "" {
				secret = existing.TokenSecret
			}
		}
		if secret == "" {
			if existing := s.syncer.GetNodePVEConfig(req.NodeID); existing != nil && existing.TokenSecret != "" {
				secret = existing.TokenSecret
			}
		}
	}
	if secret == "" && (req.NodeID == "" || req.NodeID == s.cfg.HostID) {
		secret = s.cfg.PVETokenSecret
	}

	preferredIface := req.PreferredInterface
	if preferredIface == "" {
		preferredIface = "eth0"
	}

	testClient, err := pve.NewClient(
		req.URL,
		req.TokenID,
		secret,
		req.Node,
		req.VerifySSL,
		6*time.Second,
		preferredIface,
		req.AllowedSubnets,
	)
	if err != nil {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to initialize test client: %v", err),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	if err := testClient.Ping(ctx); err != nil {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Ping failed: %v", err),
		})
		return
	}

	nodes, err := testClient.GetNodes(ctx)
	if err != nil {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"version": testClient.Version(),
			"nodes":   []string{},
			"message": fmt.Sprintf("Connected to Proxmox VE API %s", testClient.Version()),
		})
		return
	}

	// 1. Check permissions to diagnose Privilege Separation without ACL
	perms, _ := testClient.GetPermissions(ctx)
	hasZeroPerms := (perms != nil && len(perms) == 0)

	// 2. Count accessible LXCs and QEMU VMs
	clusterItems, _ := testClient.ListClusterContainers(ctx)
	lxcCount := 0
	vmCount := 0
	for _, it := range clusterItems {
		if strings.EqualFold(it.Type, "qemu") {
			vmCount++
		} else {
			lxcCount++
		}
	}
	if len(clusterItems) == 0 && len(nodes) > 0 {
		for _, n := range nodes {
			if lxcs, err := testClient.ListLXCContainers(ctx, n); err == nil {
				lxcCount += len(lxcs)
			}
			if vms, err := testClient.ListQemuVMs(ctx, n); err == nil {
				vmCount += len(vms)
			}
		}
	}

	// 3. Verify target node configuration
	var nodeWarning string
	if req.Node != "" && len(nodes) > 0 {
		matched := false
		for _, part := range strings.Split(req.Node, ",") {
			part = strings.TrimSpace(part)
			for _, on := range nodes {
				if strings.EqualFold(part, on) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			nodeWarning = fmt.Sprintf("Target node '%s' does not match detected node(s) [%s]. Leave blank to discover all nodes.", req.Node, strings.Join(nodes, ", "))
		}
	}

	var permWarning string
	if hasZeroPerms {
		permWarning = "API Token has 0 permissions (Privilege Separation is enabled). In Proxmox VE: either uncheck 'Privilege Separation' when creating the token, or go to Datacenter > Permissions > Add > API Token Permission (Path: '/', Role: 'PVEAuditor' or 'Administrator')."
	}

	msg := fmt.Sprintf("Connected successfully to Proxmox VE %s", testClient.Version())
	if len(nodes) > 0 {
		msg += fmt.Sprintf(" (Nodes: %s)", strings.Join(nodes, ", "))
	}
	msg += fmt.Sprintf(" • Visible: %d LXCs, %d VMs", lxcCount, vmCount)

	resp := map[string]interface{}{
		"success":      true,
		"version":      testClient.Version(),
		"nodes":        nodes,
		"lxc_count":    lxcCount,
		"vm_count":     vmCount,
		"has_perms":    !hasZeroPerms,
		"message":      msg,
		"warning":      permWarning,
		"node_warning": nodeWarning,
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) writeJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}
