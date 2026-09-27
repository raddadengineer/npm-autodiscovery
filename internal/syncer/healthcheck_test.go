package syncer

import (
	"context"
	"testing"
	"time"

	"github.com/sampson/npm-autodiscovery/internal/config"
	"github.com/sampson/npm-autodiscovery/internal/docker"
)

func TestShouldHoldForHealthCheck(t *testing.T) {
	s := &Syncer{
		cfg: &config.Config{},
	}

	tests := []struct {
		name     string
		inspect  *docker.ContainerInspect
		proxyCfg *ContainerProxyConfig
		wantHold bool
	}{
		{
			name: "Healthcheck disabled in proxyCfg",
			inspect: &docker.ContainerInspect{
				State: docker.ContainerState{
					Health: &docker.ContainerHealth{Status: "starting"},
				},
			},
			proxyCfg: &ContainerProxyConfig{
				HealthCheck: HealthRoutingConfig{Enabled: false},
			},
			wantHold: false,
		},
		{
			name: "No healthcheck in container inspect",
			inspect: &docker.ContainerInspect{
				State: docker.ContainerState{
					Health: nil,
				},
			},
			proxyCfg: &ContainerProxyConfig{
				HealthCheck: HealthRoutingConfig{Enabled: true},
			},
			wantHold: false,
		},
		{
			name: "Container health status starting (should hold)",
			inspect: &docker.ContainerInspect{
				State: docker.ContainerState{
					Health: &docker.ContainerHealth{Status: "starting"},
				},
			},
			proxyCfg: &ContainerProxyConfig{
				HealthCheck: HealthRoutingConfig{Enabled: true},
			},
			wantHold: true,
		},
		{
			name: "Container health status healthy (ready, should not hold)",
			inspect: &docker.ContainerInspect{
				State: docker.ContainerState{
					Health: &docker.ContainerHealth{Status: "healthy"},
				},
			},
			proxyCfg: &ContainerProxyConfig{
				HealthCheck: HealthRoutingConfig{Enabled: true},
			},
			wantHold: false,
		},
		{
			name: "Container health status unhealthy (should hold)",
			inspect: &docker.ContainerInspect{
				State: docker.ContainerState{
					Health: &docker.ContainerHealth{Status: "unhealthy"},
				},
			},
			proxyCfg: &ContainerProxyConfig{
				HealthCheck: HealthRoutingConfig{Enabled: true},
			},
			wantHold: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.shouldHoldForHealthCheck(tt.inspect, tt.proxyCfg)
			if got != tt.wantHold {
				t.Errorf("shouldHoldForHealthCheck() = %v, want %v", got, tt.wantHold)
			}
		})
	}
}

func TestQueueAndTransitionPendingHealthCheck(t *testing.T) {
	s := &Syncer{
		cfg:           &config.Config{HostID: "test-node"},
		pendingHealth: make(map[string]*PendingHealthCheck),
	}

	cID := "cid-warmup-100"
	inspect := &docker.ContainerInspect{
		ID:   cID,
		Name: "/heavy-app",
		Config: docker.ContainerConfig{
			Image: "heavy:latest",
		},
		State: docker.ContainerState{
			Running: true,
			Health:  &docker.ContainerHealth{Status: "starting"},
		},
	}
	proxyCfg := &ContainerProxyConfig{
		Enabled:       true,
		ContainerID:   cID,
		ContainerName: "heavy-app",
		DomainNames:   []string{"app.example.com"},
		ForwardScheme: "http",
		ForwardHost:   "10.0.0.5",
		ForwardPort:   8080,
		HealthCheck: HealthRoutingConfig{
			Enabled:        true,
			Timeout:        100 * time.Millisecond,
			FallbackAction: "disable",
		},
	}

	// 1. Queue container in pending
	s.queuePendingHealthCheck(context.Background(), inspect, proxyCfg, "container-ip")

	s.mu.RLock()
	pending, exists := s.pendingHealth[cID]
	s.mu.RUnlock()

	if !exists || pending == nil {
		t.Fatalf("expected container %s to be queued in pendingHealth", cID)
	}
	if pending.ContainerName != "heavy-app" {
		t.Errorf("expected container name 'heavy-app', got '%s'", pending.ContainerName)
	}

	// 2. Simulate Docker health event: health_status: healthy
	// Note: We test transition without NPM network calls by checking pending removal
	s.cancelPendingHealthCheck(cID)

	s.mu.RLock()
	_, stillPending := s.pendingHealth[cID]
	s.mu.RUnlock()

	if stillPending {
		t.Errorf("expected container %s to be cleared from pendingHealth", cID)
	}
}

func TestHealthCheckTimeoutFallback(t *testing.T) {
	s := &Syncer{
		cfg:            &config.Config{HostID: "test-node"},
		pendingHealth:  make(map[string]*PendingHealthCheck),
		trackedProxies: make(map[string]*ManagedProxy),
	}

	cID := "cid-timeout-200"
	inspect := &docker.ContainerInspect{
		ID:   cID,
		Name: "/timeout-app",
		State: docker.ContainerState{
			Running: true,
			Health:  &docker.ContainerHealth{Status: "starting"},
		},
	}
	proxyCfg := &ContainerProxyConfig{
		Enabled:       true,
		ContainerID:   cID,
		ContainerName: "timeout-app",
		DomainNames:   []string{"timeout.example.com"},
		ForwardScheme: "http",
		ForwardHost:   "10.0.0.6",
		ForwardPort:   8080,
		HealthCheck: HealthRoutingConfig{
			Enabled:        true,
			Timeout:        10 * time.Millisecond,
			FallbackAction: "disable",
		},
	}

	s.queuePendingHealthCheck(context.Background(), inspect, proxyCfg, "container-ip")

	// Wait for timeout to fire
	time.Sleep(30 * time.Millisecond)

	s.mu.RLock()
	_, exists := s.pendingHealth[cID]
	s.mu.RUnlock()

	if exists {
		t.Errorf("expected container %s to be removed from pendingHealth after timeout", cID)
	}
}
