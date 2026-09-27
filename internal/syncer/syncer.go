package syncer

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sampson/npm-autodiscovery/internal/config"
	"github.com/sampson/npm-autodiscovery/internal/docker"
	"github.com/sampson/npm-autodiscovery/internal/npm"
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
	Category  string    `json:"category"` // "discovery", "sync", "docker", "npm", "system"
	Message   string    `json:"message"`
	Details   string    `json:"details,omitempty"`
}

// ManagedProxy tracks a single auto-discovered proxy host state.
type ManagedProxy struct {
	NPMHostID        int       `json:"npm_host_id"`
	HostID           string    `json:"host_id"`
	ContainerID      string    `json:"container_id"`
	ContainerName    string    `json:"container_name"`
	Image            string    `json:"image"`
	DomainNames      []string  `json:"domain_names"`
	ForwardScheme    string    `json:"forward_scheme"`
	ForwardHost      string    `json:"forward_host"`
	ForwardPort      int       `json:"forward_port"`
	ResolutionMethod string    `json:"resolution_method"`
	SSLEnabled       bool      `json:"ssl_enabled"`
	SSLForced        bool      `json:"ssl_forced"`
	CertificateID    any       `json:"certificate_id"`
	Websocket        bool      `json:"websocket"`
	BlockExploits    bool      `json:"block_exploits"`
	Status           string    `json:"status"` // "active", "syncing", "error"
	LastSynced       time.Time `json:"last_synced"`
}

// ContainerStatusView represents running container info for the UI container explorer.
type ContainerStatusView struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	State         string            `json:"state"`
	Status        string            `json:"status"`
	Discovered    bool              `json:"discovered"`
	IgnoredReason string            `json:"ignored_reason,omitempty"`
	Domains       []string          `json:"domains,omitempty"`
	Port          int               `json:"port,omitempty"`
	Labels        map[string]string `json:"labels"`
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
	RunningContainers int       `json:"running_containers"`
	TotalEvents       int64     `json:"total_events"`
	LastFullSync      time.Time `json:"last_full_sync"`
	Uptime            int64     `json:"uptime_seconds"`
}

// Syncer is the primary coordination engine bridging Docker and NPM.
type Syncer struct {
	cfg          *config.Config
	dockerClient *docker.Client
	npmClient    *npm.Client

	mu             sync.RWMutex
	trackedProxies map[string]*ManagedProxy // Key: primary domain or container ID
	logEvents      []LogEvent
	logSubscribers map[chan LogEvent]struct{}
	logSeq         int64
	startTime      time.Time
	lastFullSync   time.Time
	totalEvents    int64
	dockerVersion  string

	syncTrigger chan struct{}
}

// NewSyncer instantiates the syncer with dependencies.
func NewSyncer(cfg *config.Config, dClient *docker.Client, nClient *npm.Client) *Syncer {
	return &Syncer{
		cfg:            cfg,
		dockerClient:   dClient,
		npmClient:      nClient,
		trackedProxies: make(map[string]*ManagedProxy),
		logEvents:      make([]LogEvent, 0, MaxLogsHistory),
		logSubscribers: make(map[chan LogEvent]struct{}),
		startTime:      time.Now(),
		syncTrigger:    make(chan struct{}, 1),
	}
}

// Start launches the background synchronization, Docker event listener, and periodic scanner.
func (s *Syncer) Start(ctx context.Context) {
	s.EmitLog(LevelInfo, "system", "NPM Auto-Discovery agent starting up", fmt.Sprintf("Poll interval: %s, Strategy: %s", s.cfg.PollInterval, s.cfg.ForwardHostStrategy))

	// Initial Docker check & version fetch
	if v, err := s.dockerClient.GetVersion(ctx); err == nil {
		s.mu.Lock()
		s.dockerVersion = fmt.Sprintf("%s (API %s)", v.Version, v.APIVersion)
		s.mu.Unlock()
		s.EmitLog(LevelSuccess, "docker", fmt.Sprintf("Connected to Docker daemon %s", s.dockerVersion), s.dockerClient.SocketPath())
	} else {
		s.EmitLog(LevelWarn, "docker", "Could not query Docker version initially", err.Error())
	}

	// Initial NPM authentication check
	if err := s.npmClient.Authenticate(ctx); err == nil {
		s.EmitLog(LevelSuccess, "npm", "Authenticated successfully with Nginx Proxy Manager", s.cfg.NPMURL)
	} else {
		s.EmitLog(LevelError, "npm", "Initial authentication with NPM failed (will retry)", err.Error())
	}

	// Run initial full sync
	s.TriggerSync()

	// 1. Docker Event Stream Goroutine
	go s.listenDockerEvents(ctx)

	// 2. Periodic Scan & Trigger Goroutine
	go s.runPeriodicSync(ctx)
}

// TriggerSync signals an immediate full sync.
func (s *Syncer) TriggerSync() {
	select {
	case s.syncTrigger <- struct{}{}:
	default:
	}
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

	switch ev.Action {
	case "start":
		s.EmitLog(LevelInfo, "docker", fmt.Sprintf("Container started: '%s'", containerName), fmt.Sprintf("ID: %s", containerID[:min(12, len(containerID))]))
		// Inspect and sync container
		s.syncContainerByID(ctx, containerID)

	case "die", "kill", "stop", "destroy":
		s.EmitLog(LevelInfo, "docker", fmt.Sprintf("Container stopped/destroyed: '%s' (action: %s)", containerName, ev.Action), fmt.Sprintf("ID: %s", containerID[:min(12, len(containerID))]))
		s.cleanupContainerProxies(ctx, containerID, containerName)
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

	s.EmitLog(LevelInfo, "discovery", fmt.Sprintf("Discovered container '%s' -> %s://%s:%d for domains %v",
		proxyCfg.ContainerName, proxyCfg.ForwardScheme, proxyCfg.ForwardHost, proxyCfg.ForwardPort, proxyCfg.DomainNames),
		fmt.Sprintf("Resolution: %s", resMethod))

	s.reconcileProxyInNPM(ctx, inspect, proxyCfg, resMethod)
}

// reconcileProxyInNPM checks if proxy host already exists, creates or updates as needed.
func (s *Syncer) reconcileProxyInNPM(ctx context.Context, inspect *docker.ContainerInspect, proxyCfg *ContainerProxyConfig, resMethod string) {
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
		AccessListID:          proxyCfg.AccessListID,
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
		isManaged, hostOwner, _, _ := getManagedInfo(matchedHost)
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

// performFullSync scans all running containers on this host and reconciles with NPM proxy hosts.
func (s *Syncer) performFullSync(ctx context.Context, triggerSource string) {
	s.EmitLog(LevelInfo, "sync", fmt.Sprintf("Running full discovery scan on host '%s' (%s)", s.cfg.HostID, triggerSource), "")

	containers, err := s.dockerClient.ListContainers(ctx, false)
	if err != nil {
		s.EmitLog(LevelError, "docker", "Full sync failed to list running containers", err.Error())
		return
	}

	runningContainerIDs := make(map[string]bool)
	runningContainerNames := make(map[string]bool)

	syncedCount := 0
	for _, c := range containers {
		runningContainerIDs[c.ID] = true
		for _, name := range c.Names {
			runningContainerNames[strings.TrimPrefix(name, "/")] = true
		}

		// Inspect container for full network and label details
		inspect, err := s.dockerClient.InspectContainer(ctx, c.ID)
		if err != nil {
			continue
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

		s.reconcileProxyInNPM(ctx, inspect, proxyCfg, resMethod)
		syncedCount++
	}

	// Orphan cleanup: ONLY prune proxy hosts created by THIS host whose containers are no longer running
	existingHosts, err := s.npmClient.GetProxyHosts(ctx)
	if err == nil {
		for _, host := range existingHosts {
			isManaged, hostOwner, cID, cName := getManagedInfo(&host)
			if isManaged {
				// Safety check for multi-host clusters: NEVER delete proxies belonging to another host!
				if hostOwner != "" && hostOwner != s.cfg.HostID {
					continue
				}

				stillRunning := false
				if cID != "" && runningContainerIDs[cID] {
					stillRunning = true
				}
				if cName != "" && runningContainerNames[cName] {
					stillRunning = true
				}

				if !stillRunning {
					s.EmitLog(LevelWarn, "sync", fmt.Sprintf("Found orphaned proxy host #%d for dead container '%s' on host '%s' (domains: %v) - pruning", host.ID, cName, s.cfg.HostID, host.DomainNames), "")
					if delErr := s.npmClient.DeleteProxyHost(ctx, host.ID); delErr == nil {
						s.EmitLog(LevelSuccess, "npm", fmt.Sprintf("Pruned orphan proxy host #%d", host.ID), "")
						s.removeTrackedProxy(cID, host.DomainNames)
					}
				}
			}
		}
	}

	s.mu.Lock()
	s.lastFullSync = time.Now()
	s.mu.Unlock()

	s.EmitLog(LevelSuccess, "sync", fmt.Sprintf("Full sync completed. Processed %d active discovered proxies across %d containers", syncedCount, len(containers)), "")
}

// saveTrackedProxy records an active managed proxy into the local state.
func (s *Syncer) saveTrackedProxy(npmID int, inspect *docker.ContainerInspect, proxyCfg *ContainerProxyConfig, resMethod, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strings.ToLower(proxyCfg.DomainNames[0])
	s.trackedProxies[key] = &ManagedProxy{
		NPMHostID:        npmID,
		HostID:           s.cfg.HostID,
		ContainerID:      proxyCfg.ContainerID,
		ContainerName:    proxyCfg.ContainerName,
		Image:            inspect.Config.Image,
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
		Status:           status,
		LastSynced:       time.Now(),
	}
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

// GetTrackedProxies returns a slice of currently tracked auto-discovered proxies.
func (s *Syncer) GetTrackedProxies() []*ManagedProxy {
	s.mu.RLock()
	defer s.mu.RUnlock()

	proxies := make([]*ManagedProxy, 0, len(s.trackedProxies))
	for _, p := range s.trackedProxies {
		proxies = append(proxies, p)
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
	dockerConn := s.dockerClient.IsConnected()
	npmConn, _, npmURL := s.npmClient.Status()
	activeCount := len(s.trackedProxies)
	totEvents := s.totalEvents
	lastSync := s.lastFullSync
	upSec := int64(time.Since(s.startTime).Seconds())
	dVer := s.dockerVersion
	hID := s.cfg.HostID
	hIP := s.cfg.HostIP
	s.mu.RUnlock()

	runningCount := 0
	if containers, err := s.dockerClient.ListContainers(ctx, false); err == nil {
		runningCount = len(containers)
	}

	return StatusOverview{
		HostID:            hID,
		HostIP:            hIP,
		DockerConnected:   dockerConn,
		DockerSocket:      s.dockerClient.SocketPath(),
		DockerVersion:     dVer,
		NPMConnected:      npmConn,
		NPMURL:            npmURL,
		ActiveProxies:     activeCount,
		RunningContainers: runningCount,
		TotalEvents:       totEvents,
		LastFullSync:      lastSync,
		Uptime:            upSec,
	}
}

// GetContainersView lists all running containers with auto-discovery evaluation.
func (s *Syncer) GetContainersView(ctx context.Context) ([]ContainerStatusView, error) {
	containers, err := s.dockerClient.ListContainers(ctx, true)
	if err != nil {
		return nil, err
	}

	views := make([]ContainerStatusView, 0, len(containers))
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
				State:         c.State,
				Status:        c.Status,
				Discovered:    false,
				IgnoredReason: "Failed to inspect container",
				Labels:        c.Labels,
			})
			continue
		}

		proxyCfg, shouldProxy, parseErr := ParseContainerLabels(inspect, s.cfg)
		discovered := shouldProxy && proxyCfg != nil
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

		views = append(views, ContainerStatusView{
			ID:            c.ID[:min(12, len(c.ID))],
			Name:          name,
			Image:         c.Image,
			State:         c.State,
			Status:        c.Status,
			Discovered:    discovered,
			IgnoredReason: ignoredReason,
			Domains:       domains,
			Port:          port,
			Labels:        c.Labels,
		})
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
	isManaged, hostOwner, cID, cName := getManagedInfo(host)
	if !isManaged {
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

// Helper: extracts managed_by metadata, host ID, and container ID/name from NPM proxy host
func getManagedInfo(host *npm.ProxyHost) (isManaged bool, hostID, containerID, containerName string) {
	if host.Meta != nil {
		if mb, ok := host.Meta["managed_by"].(string); ok && mb == ManagedByTag {
			isManaged = true
			if hid, ok := host.Meta["host_id"].(string); ok {
				hostID = hid
			}
			if id, ok := host.Meta["container_id"].(string); ok {
				containerID = id
			}
			if name, ok := host.Meta["container_name"].(string); ok {
				containerName = name
			}
			return
		}
	}
	// Fallback to checking HeaderComment in advanced_config
	if strings.Contains(host.AdvancedConfig, HeaderComment) {
		isManaged = true
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
