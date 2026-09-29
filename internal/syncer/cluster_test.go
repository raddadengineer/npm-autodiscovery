package syncer

import (
	"context"
	"testing"
	"time"

	"github.com/raddadengineer/npm-autodiscovery/internal/config"
)

func TestClusterRegistry_RegisterAndGetNodes(t *testing.T) {
	reg := NewClusterRegistry()

	localNode := ClusterNodeInfo{
		NodeID:          "main-controller",
		NodeIP:          "192.168.1.10",
		IsController:    true,
		Status:          "online",
		LastHeartbeat:   time.Now(),
		DockerConnected: true,
		ContainerCount:  3,
		ProxyCount:      2,
	}

	nodes := reg.GetNodes(localNode)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node (controller), got %d", len(nodes))
	}
	if !nodes[0].IsController || nodes[0].NodeID != "main-controller" {
		t.Errorf("unexpected controller node: %+v", nodes[0])
	}

	// Register a remote worker report
	workerReport := NodeReport{
		NodeID:     "worker-01",
		NodeIP:     "192.168.1.20",
		ReportedAt: time.Now(),
		Status: StatusOverview{
			HostID:            "worker-01",
			HostIP:            "192.168.1.20",
			DockerConnected:   true,
			RunningContainers: 2,
			ActiveProxies:     1,
			Uptime:            3600,
		},
		Containers: []ContainerStatusView{
			{
				ID:         "c123456",
				Name:       "worker-whoami",
				Image:      "traefik/whoami",
				State:      "running",
				Discovered: true,
				Domains:    []string{"worker.local"},
				Port:       80,
			},
		},
		Proxies: []*ManagedProxy{
			{
				NPMHostID:     101,
				HostID:        "worker-01",
				ContainerID:   "c123456",
				ContainerName: "worker-whoami",
				DomainNames:   []string{"worker.local"},
				ForwardHost:   "192.168.1.20",
				ForwardPort:   8081,
				Status:        "active",
			},
		},
		Streams: []*ManagedStream{
			{
				NPMStreamID:    201,
				HostID:         "worker-01",
				IncomingPort:   25565,
				ForwardingHost: "192.168.1.20",
				ForwardingPort: 25565,
				TCPForwarding:  true,
				Status:         "active",
			},
		},
	}

	reg.RegisterOrUpdate(workerReport)

	nodes = reg.GetNodes(localNode)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes (controller + worker), got %d", len(nodes))
	}

	worker := nodes[1]
	if worker.NodeID != "worker-01" || worker.IsController || worker.Status != "online" {
		t.Errorf("unexpected worker node data: %+v", worker)
	}

	// Verify containers aggregation
	remoteContainers := reg.GetRemoteContainers()
	if len(remoteContainers) != 1 {
		t.Fatalf("expected 1 remote container, got %d", len(remoteContainers))
	}
	if remoteContainers[0].NodeID != "worker-01" {
		t.Errorf("expected container NodeID to be 'worker-01', got '%s'", remoteContainers[0].NodeID)
	}

	// Verify proxies aggregation
	remoteProxies := reg.GetRemoteProxies()
	if len(remoteProxies) != 1 {
		t.Fatalf("expected 1 remote proxy, got %d", len(remoteProxies))
	}
	if remoteProxies[0].HostID != "worker-01" {
		t.Errorf("expected proxy HostID to be 'worker-01', got '%s'", remoteProxies[0].HostID)
	}

	// Verify streams aggregation
	remoteStreams := reg.GetRemoteStreams()
	if len(remoteStreams) != 1 {
		t.Fatalf("expected 1 remote stream, got %d", len(remoteStreams))
	}
	if remoteStreams[0].HostID != "worker-01" {
		t.Errorf("expected stream HostID to be 'worker-01', got '%s'", remoteStreams[0].HostID)
	}
}

func TestSyncer_ClusterIntegration(t *testing.T) {
	cfg := &config.Config{
		HostID:  "main-controller",
		HostIP:  "192.168.1.10",
		Port:    8080,
		NPMURL:  "http://127.0.0.1:81",
		NPMUser: "test@example.com",
	}

	s := NewSyncer(cfg, nil, nil)

	// Local state
	s.mu.Lock()
	s.trackedProxies["local.test"] = &ManagedProxy{
		NPMHostID:   1,
		HostID:      "main-controller",
		DomainNames: []string{"local.test"},
		ForwardHost: "127.0.0.1",
		ForwardPort: 8080,
	}
	s.mu.Unlock()

	// Register worker report
	err := s.RegisterNodeReport(NodeReport{
		NodeID:     "worker-edge-02",
		NodeIP:     "192.168.1.22",
		ReportedAt: time.Now(),
		Status: StatusOverview{
			HostID: "worker-edge-02",
			Uptime: 1200,
		},
		Containers: []ContainerStatusView{
			{
				ID:         "c999",
				Name:       "edge-app",
				Discovered: true,
				Domains:    []string{"edge.example.com"},
				NodeID:     "worker-edge-02",
			},
		},
		Proxies: []*ManagedProxy{
			{
				NPMHostID:   2,
				HostID:      "worker-edge-02",
				DomainNames: []string{"edge.example.com"},
				ForwardHost: "192.168.1.22",
				ForwardPort: 9000,
			},
		},
	})

	if err != nil {
		t.Fatalf("failed to register node report: %v", err)
	}

	// Verify all proxies includes both local and worker
	proxies := s.GetTrackedProxies()
	if len(proxies) != 2 {
		t.Fatalf("expected 2 tracked proxies across cluster, got %d", len(proxies))
	}

	// Verify all containers includes both local and worker
	containers, _ := s.GetContainersView(context.Background())
	if len(containers) != 1 {
		t.Fatalf("expected 1 container from worker, got %d", len(containers))
	}
	if containers[0].NodeID != "worker-edge-02" {
		t.Errorf("expected container NodeID to be worker-edge-02, got %s", containers[0].NodeID)
	}

	// Check cluster nodes view
	nodes := s.GetClusterNodes()
	if len(nodes) != 2 {
		t.Fatalf("expected 2 cluster nodes, got %d", len(nodes))
	}
	if nodes[0].NodeID != "main-controller" || !nodes[0].IsController {
		t.Errorf("unexpected controller node: %+v", nodes[0])
	}
	if nodes[1].NodeID != "worker-edge-02" || nodes[1].IsController {
		t.Errorf("unexpected worker node: %+v", nodes[1])
	}
}

func TestClusterRegistry_PersistenceAndUpgradeResilience(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("DATA_DIR", tempDir)

	reg1 := NewClusterRegistry()

	// 1. Save config under "worker-node-01"
	reg1.SetNodePVEConfig("worker-node-01", PVEConfig{
		ID:          "pve-1",
		Name:        "Proxmox Production",
		Enabled:     true,
		URL:         "https://pve.example.com:8006",
		TokenID:     "root@pam!token",
		TokenSecret: "super-secret-token",
		Node:        "pve-node-1",
	})

	// Verify it saved
	cfgs := reg1.GetNodePVEConfigs("worker-node-01")
	if len(cfgs) != 1 || cfgs[0].TokenSecret != "super-secret-token" {
		t.Fatalf("expected 1 config with secret, got %+v", cfgs)
	}

	// 2. Simulate upgrade: create a brand new registry instance pointing to the same DATA_DIR
	reg2 := NewClusterRegistry()

	// Verify exact match
	cfgs2 := reg2.GetNodePVEConfigs("worker-node-01")
	if len(cfgs2) != 1 || cfgs2[0].TokenSecret != "super-secret-token" {
		t.Fatalf("expected config to survive reload, got %+v", cfgs2)
	}

	// Verify case-insensitive match
	cfgsCase := reg2.GetNodePVEConfigs("WORKER-NODE-01")
	if len(cfgsCase) != 1 || cfgsCase[0].ID != "pve-1" {
		t.Fatalf("expected case-insensitive match, got %+v", cfgsCase)
	}

	// Verify single-node fallback: if querying for "controller-main" or "local", it falls back to the 1 saved node config
	cfgsFallback := reg2.GetNodePVEConfigs("controller-main")
	if len(cfgsFallback) != 1 || cfgsFallback[0].ID != "pve-1" {
		t.Fatalf("expected single-node fallback to return saved config, got %+v", cfgsFallback)
	}

	// 3. Test secret preservation: UI updates endpoint name/URL without re-sending secret
	reg2.SetNodePVEConfig("worker-node-01", PVEConfig{
		ID:      "pve-1",
		Name:    "Proxmox Production Renamed",
		Enabled: true,
		URL:     "https://pve-renamed.example.com:8006",
		TokenID: "root@pam!token",
		// TokenSecret intentionally empty (as submitted by dashboard UI)
	})

	cfgsUpdated := reg2.GetNodePVEConfigs("worker-node-01")
	if len(cfgsUpdated) != 1 {
		t.Fatalf("expected 1 config, got %d", len(cfgsUpdated))
	}
	if cfgsUpdated[0].TokenSecret != "super-secret-token" {
		t.Errorf("expected TokenSecret to be preserved, got '%s'", cfgsUpdated[0].TokenSecret)
	}
	if !cfgsUpdated[0].HasSecret {
		t.Errorf("expected HasSecret to be true")
	}
	if cfgsUpdated[0].Name != "Proxmox Production Renamed" {
		t.Errorf("expected updated name, got '%s'", cfgsUpdated[0].Name)
	}
}

func TestClusterRegistry_DeleteDefaultEndpointAndTelemetryNode(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("DATA_DIR", tempDir)

	reg := NewClusterRegistry()

	// Case 1: Worker node "monitor" registers telemetry report with PVE enabled but no saved pveConfigs
	reg.RegisterOrUpdate(NodeReport{
		NodeID: "monitor",
		NodeIP: "192.168.1.150",
		Status: StatusOverview{
			PVEEnabled:   true,
			PVEConnected: true,
			PVEURL:       "https://monitor-pve:8006",
			PVENode:      "monitor",
		},
	})

	// GetNodePVEConfigs should see the telemetry-backed default endpoint
	cfgs := reg.GetNodePVEConfigs("monitor")
	if len(cfgs) != 1 || cfgs[0].ID != "default" {
		t.Fatalf("expected 1 default config from telemetry, got %+v", cfgs)
	}

	// Deleting config "default" on node "monitor" must succeed
	deleted := reg.DeleteNodePVEConfig("monitor", "default")
	if !deleted {
		t.Fatalf("expected DeleteNodePVEConfig('monitor', 'default') to return true")
	}

	// After deletion, GetNodePVEConfigs must return nil
	cfgsAfter := reg.GetNodePVEConfigs("monitor")
	if len(cfgsAfter) != 0 {
		t.Fatalf("expected 0 configs after deletion, got %+v", cfgsAfter)
	}

	// HasExplicitEmptyPVEConfigs should be true
	if !reg.HasExplicitEmptyPVEConfigs("monitor") {
		t.Fatalf("expected HasExplicitEmptyPVEConfigs('monitor') to be true")
	}

	// Verify GetNodes returns monitor as PVEEnabled=false and PVEConnected=false
	nodes := reg.GetNodes(ClusterNodeInfo{NodeID: "controller-main", IsController: true})
	for _, n := range nodes {
		if n.NodeID == "monitor" {
			if n.PVEConnected {
				t.Fatalf("expected monitor.PVEConnected to be false after deletion, got true")
			}
			if n.PVEEnabled {
				t.Fatalf("expected monitor.PVEEnabled to be false after deletion, got true")
			}
		}
	}

	// Heartbeat from remote worker still reporting PVEConnected: true must be overridden by registry
	reg.RegisterOrUpdate(NodeReport{
		NodeID: "monitor",
		NodeIP: "192.168.1.150",
		Status: StatusOverview{
			PVEEnabled:   true,
			PVEConnected: true,
			PVEURL:       "https://monitor-pve:8006",
			PVENode:      "monitor",
		},
	})
	nodesAfterHeartbeat := reg.GetNodes(ClusterNodeInfo{NodeID: "controller-main", IsController: true})
	for _, n := range nodesAfterHeartbeat {
		if n.NodeID == "monitor" {
			if n.PVEConnected {
				t.Fatalf("expected monitor.PVEConnected to be false after heartbeat override, got true")
			}
			if n.PVEEnabled {
				t.Fatalf("expected monitor.PVEEnabled to be false after heartbeat override, got true")
			}
		}
	}

	// Case 2: Node has sole endpoint with random generated ID like "pve-1790563574911"
	reg.SetNodePVEConfig("worker-node-02", PVEConfig{
		ID:      "pve-1790563574911",
		Name:    "Proxmox Random",
		Enabled: true,
		URL:     "https://pve2:8006",
	})

	// Deleting with configID "default" on single-endpoint node must succeed
	deleted = reg.DeleteNodePVEConfig("worker-node-02", "default")
	if !deleted {
		t.Fatalf("expected DeleteNodePVEConfig with 'default' on sole endpoint to succeed")
	}
	if len(reg.GetNodePVEConfigs("worker-node-02")) != 0 {
		t.Fatalf("expected 0 configs after sole endpoint deletion")
	}

	// Case 3: Idempotent deletion on empty node must succeed
	deleted = reg.DeleteNodePVEConfig("non-existent-node", "default")
	if !deleted {
		t.Fatalf("expected idempotent deletion of 'default' to return true")
	}
}

func TestDeleteNodeAndPruneOfflineNodes(t *testing.T) {
	reg := NewClusterRegistry()

	// 1. Register an active online node
	reg.RegisterOrUpdate(NodeReport{
		NodeID:     "worker-online",
		NodeIP:     "192.168.1.100",
		ReportedAt: time.Now(),
		Status: StatusOverview{
			Uptime:          100,
			DockerConnected: true,
		},
	})

	// 2. Register an offline node (heartbeat 60s ago)
	reg.RegisterOrUpdate(NodeReport{
		NodeID:     "worker-offline-1",
		NodeIP:     "192.168.1.101",
		ReportedAt: time.Now().Add(-60 * time.Second),
		Status: StatusOverview{
			Uptime:          50,
			DockerConnected: false,
		},
	})
	// Manually set LastHeartbeat in reg.nodes to 60s ago to simulate offline
	reg.mu.Lock()
	if n, ok := reg.nodes["worker-offline-1"]; ok {
		n.LastHeartbeat = time.Now().Add(-60 * time.Second)
	}
	reg.mu.Unlock()

	// 3. Register a second offline node
	reg.RegisterOrUpdate(NodeReport{
		NodeID:     "worker-offline-2",
		NodeIP:     "192.168.1.102",
		ReportedAt: time.Now().Add(-120 * time.Second),
	})
	reg.mu.Lock()
	if n, ok := reg.nodes["worker-offline-2"]; ok {
		n.LastHeartbeat = time.Now().Add(-120 * time.Second)
	}
	reg.mu.Unlock()

	// Set PVE config for worker-offline-1
	reg.SetNodePVEConfig("worker-offline-1", PVEConfig{
		ID:      "pve-1",
		Enabled: true,
		URL:     "https://pve-offline:8006",
	})

	// 4. Attempt to delete online node without force -> should fail
	err := reg.DeleteNode("worker-online", false)
	if err == nil {
		t.Fatalf("expected error deleting online node without force, got nil")
	}

	// 5. Delete online node with force -> should succeed
	err = reg.DeleteNode("worker-online", true)
	if err != nil {
		t.Fatalf("expected successful deletion with force=true, got: %v", err)
	}

	// 6. Attempt to delete non-existent node -> should fail
	err = reg.DeleteNode("unknown-node", false)
	if err == nil {
		t.Fatalf("expected error deleting unknown node, got nil")
	}

	// 7. Delete worker-offline-1
	err = reg.DeleteNode("worker-offline-1", false)
	if err != nil {
		t.Fatalf("expected successful deletion of offline node, got: %v", err)
	}
	// Verify PVE config was also cleaned up
	if len(reg.GetNodePVEConfigs("worker-offline-1")) != 0 {
		t.Fatalf("expected PVE config to be removed for deleted node")
	}

	// 8. Test DeleteOfflineNodes with worker-offline-2 still remaining
	removed := reg.DeleteOfflineNodes()
	if len(removed) != 1 || removed[0] != "worker-offline-2" {
		t.Fatalf("expected DeleteOfflineNodes to return ['worker-offline-2'], got: %v", removed)
	}

	// Verify registry is now empty of remote nodes
	localNode := ClusterNodeInfo{NodeID: "controller-main", IsController: true, Status: "online"}
	allNodes := reg.GetNodes(localNode)
	if len(allNodes) != 1 {
		t.Fatalf("expected only controller node to remain, got %d nodes", len(allNodes))
	}
}


