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
