package syncer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sampson/npm-autodiscovery/internal/config"
	"github.com/sampson/npm-autodiscovery/internal/docker"
)

// ResolveForwardHost determines the target IP or hostname that NPM should proxy requests to.
func ResolveForwardHost(c *docker.ContainerInspect, proxyCfg *ContainerProxyConfig, cfg *config.Config) (string, string, error) {
	// 1. Explicit label override (e.g., npm.forward_host=10.0.0.5 or npm.target_host=my-service)
	if proxyCfg.ForwardHost != "" {
		return proxyCfg.ForwardHost, "label-override", nil
	}

	containerName := strings.TrimPrefix(c.Name, "/")
	networks := c.NetworkSettings.Networks

	// 2. Multi-Host / Remote Node Mode: HOST_IP / NODE_IP specified
	// When this agent is running on a different Docker host from NPM, NPM routes to the host's IP
	if cfg.HostIP != "" {
		// If configured to use host ports, map the container port to the published host port
		if cfg.UseHostPort && c.NetworkSettings.Ports != nil {
			targetProtoKey := fmt.Sprintf("%d/tcp", proxyCfg.ForwardPort)
			if bindings, ok := c.NetworkSettings.Ports[targetProtoKey]; ok && len(bindings) > 0 {
				if hostP, err := strconv.Atoi(bindings[0].HostPort); err == nil && hostP > 0 {
					proxyCfg.ForwardPort = hostP
				}
			} else {
				// If no direct port key match, check first available published host port
				for _, bindingsList := range c.NetworkSettings.Ports {
					if len(bindingsList) > 0 {
						if hostP, err := strconv.Atoi(bindingsList[0].HostPort); err == nil && hostP > 0 {
							proxyCfg.ForwardPort = hostP
							break
						}
					}
				}
			}
		}

		return cfg.HostIP, fmt.Sprintf("remote-host-ip (%s)", cfg.HostIP), nil
	}

	// 3. Specific NPM_NETWORK configured (same Docker host / shared bridge)
	if cfg.NPMNetwork != "" {
		if netSettings, exists := networks[cfg.NPMNetwork]; exists {
			switch cfg.ForwardHostStrategy {
			case "name":
				return containerName, fmt.Sprintf("network-name (%s)", cfg.NPMNetwork), nil
			case "ip":
				if netSettings.IPAddress != "" {
					return netSettings.IPAddress, fmt.Sprintf("network-ip (%s)", cfg.NPMNetwork), nil
				}
			case "auto":
				// When sharing a dedicated network, container name resolution via Docker DNS is best practice
				return containerName, fmt.Sprintf("shared-network-dns (%s)", cfg.NPMNetwork), nil
			}
		}
	}

	// 4. Fallback based on Strategy
	switch cfg.ForwardHostStrategy {
	case "name":
		return containerName, "container-name", nil

	case "ip":
		// Find first network with an assigned IP
		for netName, netSettings := range networks {
			if netSettings.IPAddress != "" {
				return netSettings.IPAddress, fmt.Sprintf("container-ip (%s)", netName), nil
			}
		}
		if c.NetworkSettings.IPAddress != "" {
			return c.NetworkSettings.IPAddress, "bridge-ip", nil
		}
		return "", "", fmt.Errorf("could not determine container IP address for %s", containerName)

	case "auto":
		// Check if container is on any non-default network (user-defined bridge)
		for netName, netSettings := range networks {
			if netName != "bridge" && netName != "host" && netName != "none" {
				// Container is on a user-defined network.
				// Prefer container name if Docker internal DNS is active, but check IP availability
				if netSettings.IPAddress != "" {
					return containerName, fmt.Sprintf("user-network (%s)", netName), nil
				}
			}
		}

		// Fallback to container IP from any network
		for netName, netSettings := range networks {
			if netSettings.IPAddress != "" {
				return netSettings.IPAddress, fmt.Sprintf("ip (%s)", netName), nil
			}
		}
		if c.NetworkSettings.IPAddress != "" {
			return c.NetworkSettings.IPAddress, "bridge-ip", nil
		}

		// Final fallback: container name
		return containerName, "container-name-fallback", nil
	}

	return containerName, "default-fallback", nil
}
