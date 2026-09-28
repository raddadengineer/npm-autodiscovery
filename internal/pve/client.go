package pve

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Client is a Proxmox VE REST API client.
type Client struct {
	baseURL            string
	tokenID            string
	tokenSecret        string
	node               string
	preferredInterface string
	allowedSubnets     []*net.IPNet
	httpClient         *http.Client

	mu        sync.RWMutex
	connected bool
	version   string
	lastPing  time.Time
}

// NewClient initializes a Proxmox VE API client with token authorization.
func NewClient(
	rawURL string,
	tokenID string,
	tokenSecret string,
	node string,
	verifySSL bool,
	timeout time.Duration,
	preferredInterface string,
	allowedSubnetsStr string,
) (*Client, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("pve: baseURL cannot be empty")
	}

	cleanURL := strings.TrimRight(rawURL, "/")
	if !strings.HasPrefix(cleanURL, "http://") && !strings.HasPrefix(cleanURL, "https://") {
		cleanURL = "https://" + cleanURL
	}

	subnets, err := ParseSubnets(allowedSubnetsStr)
	if err != nil {
		return nil, fmt.Errorf("pve: %w", err)
	}

	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	if preferredInterface == "" {
		preferredInterface = "eth0"
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !verifySSL,
		},
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression: true,
	}

	return &Client{
		baseURL:            cleanURL,
		tokenID:            tokenID,
		tokenSecret:        tokenSecret,
		node:               node,
		preferredInterface: preferredInterface,
		allowedSubnets:     subnets,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   timeout,
		},
	}, nil
}

// doRequest executes an authenticated HTTP request to the Proxmox VE API.
func (c *Client) doRequest(ctx context.Context, method, endpoint string) ([]byte, error) {
	reqURL := fmt.Sprintf("%s%s", c.baseURL, endpoint)
	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("pve request create failed: %w", err)
	}

	// Proxmox API Token authentication format:
	// Authorization: PVEAPIToken=USER@REALM!TOKENID=SECRET
	if c.tokenID != "" && c.tokenSecret != "" {
		tokenHeader := fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenID, c.tokenSecret)
		req.Header.Set("Authorization", tokenHeader)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		return nil, fmt.Errorf("pve connection error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pve reading response failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pve api error (status %d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// Ping checks whether Proxmox VE API is responding and updates connected state.
func (c *Client) Ping(ctx context.Context) error {
	data, err := c.doRequest(ctx, http.MethodGet, "/api2/json/version")
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastPing = time.Now()
	if err != nil {
		c.connected = false
		return err
	}

	var res APIResponse
	if err := json.Unmarshal(data, &res); err != nil {
		c.connected = false
		return err
	}

	var v VersionInfo
	if err := json.Unmarshal(res.Data, &v); err == nil && v.Version != "" {
		c.version = fmt.Sprintf("%s (%s)", v.Version, v.Release)
	}

	c.connected = true
	return nil
}

// IsConnected returns whether Proxmox VE is connected.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Version returns the Proxmox VE version string.
func (c *Client) Version() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.version
}

// BaseURL returns the configured Proxmox base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// ConfiguredNode returns the configured node name or empty for cluster.
func (c *Client) ConfiguredNode() string {
	return c.node
}

// GetNodes returns the list of online nodes in the Proxmox cluster.
func (c *Client) GetNodes(ctx context.Context) ([]string, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/api2/json/nodes")
	if err != nil {
		return nil, err
	}

	var res APIResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	var nodes []NodeSummary
	if err := json.Unmarshal(res.Data, &nodes); err != nil {
		return nil, err
	}

	var onlineNodes []string
	for _, n := range nodes {
		if strings.EqualFold(n.Status, "online") || n.Status == "" {
			onlineNodes = append(onlineNodes, n.Node)
		}
	}
	return onlineNodes, nil
}

// ListLXCContainers lists LXC containers on a specific node.
func (c *Client) ListLXCContainers(ctx context.Context, node string) ([]LXCContainerSummary, error) {
	endpoint := fmt.Sprintf("/api2/json/nodes/%s/lxc", node)
	data, err := c.doRequest(ctx, http.MethodGet, endpoint)
	if err != nil {
		return nil, err
	}

	var res APIResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	var containers []LXCContainerSummary
	if err := json.Unmarshal(res.Data, &containers); err != nil {
		return nil, err
	}

	for i := range containers {
		containers[i].Node = node
	}
	return containers, nil
}

// GetLXCConfig retrieves the container configuration including Notes (description) and network config.
func (c *Client) GetLXCConfig(ctx context.Context, node string, vmid int) (*LXCConfig, error) {
	endpoint := fmt.Sprintf("/api2/json/nodes/%s/lxc/%d/config", node, vmid)
	data, err := c.doRequest(ctx, http.MethodGet, endpoint)
	if err != nil {
		return nil, err
	}

	var res APIResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	var cfg LXCConfig
	if err := json.Unmarshal(res.Data, &cfg); err != nil {
		return nil, err
	}

	// Also populate raw map to inspect any netX
	var raw map[string]interface{}
	if err := json.Unmarshal(res.Data, &raw); err == nil {
		cfg.Raw = raw
	}

	return &cfg, nil
}

// GetLXCInterfaces queries active network interfaces for a container.
func (c *Client) GetLXCInterfaces(ctx context.Context, node string, vmid int) ([]NetworkInterface, error) {
	endpoint := fmt.Sprintf("/api2/json/nodes/%s/lxc/%d/interfaces", node, vmid)
	data, err := c.doRequest(ctx, http.MethodGet, endpoint)
	if err != nil {
		return nil, err
	}

	var res APIResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	var ifaces []NetworkInterface
	if err := json.Unmarshal(res.Data, &ifaces); err != nil {
		return nil, err
	}

	return ifaces, nil
}

// ResolveContainerIP resolves the container's IP address.
// Precedence:
// 1. Preferred interface from /interfaces
// 2. Any other non-loopback interface matching allowed subnets
// 3. Fallback to net0...net3 config in LXCConfig
func (c *Client) ResolveContainerIP(ctx context.Context, node string, vmid int, cfg *LXCConfig) (string, string, error) {
	// 1. Try querying active interfaces
	ifaces, err := c.GetLXCInterfaces(ctx, node, vmid)
	if err == nil && len(ifaces) > 0 {
		// First pass: look for preferred interface
		for _, iface := range ifaces {
			if strings.EqualFold(iface.Name, c.preferredInterface) && iface.Inet != "" {
				if cleanIP, ipObj, err := ExtractCleanIPv4(iface.Inet); err == nil {
					if MatchSubnets(ipObj, c.allowedSubnets) {
						return cleanIP, iface.Name, nil
					}
				}
			}
		}

		// Second pass: any valid non-loopback interface
		for _, iface := range ifaces {
			if strings.EqualFold(iface.Name, "lo") {
				continue
			}
			if iface.Inet != "" {
				if cleanIP, ipObj, err := ExtractCleanIPv4(iface.Inet); err == nil {
					if MatchSubnets(ipObj, c.allowedSubnets) {
						return cleanIP, iface.Name, nil
					}
				}
			}
		}
	}

	// 2. Fallback to LXC configuration netX fields
	if cfg != nil {
		netFields := []string{cfg.Net0, cfg.Net1, cfg.Net2, cfg.Net3}
		if cfg.Raw != nil {
			for i := 4; i < 10; i++ {
				key := fmt.Sprintf("net%d", i)
				if val, ok := cfg.Raw[key].(string); ok && val != "" {
					netFields = append(netFields, val)
				}
			}
		}

		// First pass: match preferred interface
		for _, netStr := range netFields {
			if netStr == "" {
				continue
			}
			ifaceName, rawIP := ParseNetConfig(netStr)
			if strings.EqualFold(ifaceName, c.preferredInterface) && rawIP != "" {
				if cleanIP, ipObj, err := ExtractCleanIPv4(rawIP); err == nil {
					if MatchSubnets(ipObj, c.allowedSubnets) {
						return cleanIP, ifaceName, nil
					}
				}
			}
		}

		// Second pass: any valid interface
		for _, netStr := range netFields {
			if netStr == "" {
				continue
			}
			ifaceName, rawIP := ParseNetConfig(netStr)
			if rawIP != "" {
				if cleanIP, ipObj, err := ExtractCleanIPv4(rawIP); err == nil {
					if MatchSubnets(ipObj, c.allowedSubnets) {
						return cleanIP, ifaceName, nil
					}
				}
			}
		}
	}

	return "", "", fmt.Errorf("no valid IPv4 address found for LXC %d on node %s", vmid, node)
}

// DiscoverRoutes scans the configured node or all cluster nodes and discovers routes and streams.
func (c *Client) DiscoverRoutes(
	ctx context.Context,
	defaultScheme string,
	defaultSSL bool,
	defaultWebsocket bool,
	defaultBlockExploits bool,
) ([]PVERoute, []PVEStream, []LXCContainerSummary, error) {
	var targetNodes []string
	if strings.TrimSpace(c.node) != "" {
		for _, n := range strings.Split(c.node, ",") {
			n = strings.TrimSpace(n)
			if n != "" {
				targetNodes = append(targetNodes, n)
			}
		}
	}
	if len(targetNodes) == 0 {
		onlineNodes, err := c.GetNodes(ctx)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed listing nodes: %w", err)
		}
		targetNodes = onlineNodes
	}

	var allRoutes []PVERoute
	var allStreams []PVEStream
	var allContainers []LXCContainerSummary

	for _, nodeName := range targetNodes {
		containers, err := c.ListLXCContainers(ctx, nodeName)
		if err != nil {
			continue
		}

		for _, cont := range containers {
			allContainers = append(allContainers, cont)

			// Only process running containers
			if !strings.EqualFold(cont.Status, "running") {
				continue
			}

			// Get container configuration (for Notes and net fallback)
			cfg, err := c.GetLXCConfig(ctx, nodeName, cont.VMID)
			if err != nil {
				continue
			}

			// Resolve container IP
			ip, iface, err := c.ResolveContainerIP(ctx, nodeName, cont.VMID, cfg)
			if err != nil || ip == "" {
				continue
			}

			notes := ""
			if cfg != nil {
				notes = cfg.Description
			}

			tags := cont.Tags
			if tags == "" && cfg != nil {
				tags = cfg.Tags
			}

			route, streams, ok := ExtractRouteAndStreams(
				cont.VMID,
				nodeName,
				cont.Name,
				ip,
				iface,
				tags,
				notes,
				defaultScheme,
				defaultSSL,
				defaultWebsocket,
				defaultBlockExploits,
			)

			if !ok {
				continue
			}

			if route != nil {
				allRoutes = append(allRoutes, *route)
			}
			if len(streams) > 0 {
				allStreams = append(allStreams, streams...)
			}
		}
	}

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	return allRoutes, allStreams, allContainers, nil
}
