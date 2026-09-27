package syncer

import (
	"testing"

	"github.com/raddadengineer/npm-autodiscovery/internal/config"
	"github.com/raddadengineer/npm-autodiscovery/internal/docker"
)

func TestResolveForwardHost(t *testing.T) {
	container := &docker.ContainerInspect{
		ID:   "container123456",
		Name: "/my-api-service",
		NetworkSettings: docker.NetworkSettings{
			IPAddress: "172.17.0.2",
			Networks: map[string]docker.NetworkEndpointSettings{
				"bridge": {
					IPAddress: "172.17.0.2",
				},
				"npm-network": {
					IPAddress: "172.20.0.5",
				},
			},
		},
	}

	t.Run("Explicit label override", func(t *testing.T) {
		cfg := &config.Config{ForwardHostStrategy: "auto"}
		proxyCfg := &ContainerProxyConfig{ForwardHost: "192.168.1.100"}
		host, method, err := ResolveForwardHost(container, proxyCfg, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if host != "192.168.1.100" || method != "label-override" {
			t.Errorf("got (%s, %s), want (192.168.1.100, label-override)", host, method)
		}
	})

	t.Run("NPM Network with name strategy", func(t *testing.T) {
		cfg := &config.Config{
			NPMNetwork:          "npm-network",
			ForwardHostStrategy: "name",
		}
		proxyCfg := &ContainerProxyConfig{}
		host, method, err := ResolveForwardHost(container, proxyCfg, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if host != "my-api-service" {
			t.Errorf("got host %s, want my-api-service", host)
		}
		if method != "network-name (npm-network)" {
			t.Errorf("got method %s, want network-name (npm-network)", method)
		}
	})

	t.Run("NPM Network with ip strategy", func(t *testing.T) {
		cfg := &config.Config{
			NPMNetwork:          "npm-network",
			ForwardHostStrategy: "ip",
		}
		proxyCfg := &ContainerProxyConfig{}
		host, _, err := ResolveForwardHost(container, proxyCfg, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if host != "172.20.0.5" {
			t.Errorf("got host %s, want 172.20.0.5", host)
		}
	})
}
