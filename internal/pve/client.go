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
		if containers[i].Type == "" {
			containers[i].Type = "lxc"
		}
	}
	return containers, nil
}

// ListQemuVMs lists QEMU virtual machines on a specific node.
func (c *Client) ListQemuVMs(ctx context.Context, node string) ([]LXCContainerSummary, error) {
	endpoint := fmt.Sprintf("/api2/json/nodes/%s/qemu", node)
	data, err := c.doRequest(ctx, http.MethodGet, endpoint)
	if err != nil {
		return nil, err
	}

	var res APIResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	var vms []LXCContainerSummary
	if err := json.Unmarshal(res.Data, &vms); err != nil {
		return nil, err
	}

	for i := range vms {
		vms[i].Node = node
		if vms[i].Type == "" {
			vms[i].Type = "qemu"
		}
	}
	return vms, nil
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

// GetQemuConfig retrieves the VM configuration including Notes (description) and network config.
func (c *Client) GetQemuConfig(ctx context.Context, node string, vmid int) (*LXCConfig, error) {
	endpoint := fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/config", node, vmid)
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

// GetQemuInterfaces queries active network interfaces via QEMU Guest Agent.
func (c *Client) GetQemuInterfaces(ctx context.Context, node string, vmid int) ([]NetworkInterface, error) {
	endpoint := fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/agent/network-get-interfaces", node, vmid)
	data, err := c.doRequest(ctx, http.MethodGet, endpoint)
	if err != nil {
		return nil, err
	}

	var res APIResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	var agentRes struct {
		Result []struct {
			Name        string `json:"name"`
			IPAddresses []struct {
				IPAddress     string `json:"ip-address"`
				IPAddressType string `json:"ip-address-type"`
				Prefix        int    `json:"prefix"`
			} `json:"ip-addresses"`
		} `json:"result"`
	}

	if err := json.Unmarshal(res.Data, &agentRes); err != nil {
		return nil, err
	}

	var ifaces []NetworkInterface
	for _, r := range agentRes.Result {
		for _, ipInfo := range r.IPAddresses {
			if strings.EqualFold(ipInfo.IPAddressType, "ipv4") {
				ifaces = append(ifaces, NetworkInterface{
					Name: r.Name,
					Inet: fmt.Sprintf("%s/%d", ipInfo.IPAddress, ipInfo.Prefix),
				})
			}
		}
	}

	return ifaces, nil
}

// GetPermissions queries Proxmox VE permissions for current token/user.
func (c *Client) GetPermissions(ctx context.Context) (map[string]interface{}, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/api2/json/access/permissions")
	if err != nil {
		return nil, err
	}

	var res APIResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	var perms map[string]interface{}
	if err := json.Unmarshal(res.Data, &perms); err != nil {
		return nil, err
	}

	return perms, nil
}

// ResolveContainerIP resolves the container's IP address.
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

// ResolveQemuIP resolves the IP address for a QEMU virtual machine.
func (c *Client) ResolveQemuIP(ctx context.Context, node string, vmid int, cfg *LXCConfig) (string, string, error) {
	// 1. Try querying active interfaces via QEMU Guest Agent
	ifaces, err := c.GetQemuInterfaces(ctx, node, vmid)
	if err == nil && len(ifaces) > 0 {
		for _, iface := range ifaces {
			if strings.EqualFold(iface.Name, c.preferredInterface) && iface.Inet != "" {
				if cleanIP, ipObj, err := ExtractCleanIPv4(iface.Inet); err == nil {
					if MatchSubnets(ipObj, c.allowedSubnets) {
						return cleanIP, iface.Name, nil
					}
				}
			}
		}

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

	// 2. Fallback to Cloud-Init ipconfig0...ipconfig3
	if cfg != nil && cfg.Raw != nil {
		for i := 0; i < 4; i++ {
			key := fmt.Sprintf("ipconfig%d", i)
			if val, ok := cfg.Raw[key].(string); ok && val != "" {
				// Format: ip=192.168.1.150/24,gw=192.168.1.1
				for _, part := range strings.Split(val, ",") {
					part = strings.TrimSpace(part)
					if strings.HasPrefix(part, "ip=") {
						rawIP := strings.TrimPrefix(part, "ip=")
						if rawIP != "dhcp" && rawIP != "" {
							if cleanIP, ipObj, err := ExtractCleanIPv4(rawIP); err == nil {
								if MatchSubnets(ipObj, c.allowedSubnets) {
									return cleanIP, fmt.Sprintf("ipconfig%d", i), nil
								}
							}
						}
					}
				}
			}
		}
	}

	return "", "", fmt.Errorf("no valid IPv4 address found for QEMU VM %d on node %s", vmid, node)
}

// ResolveInstanceIP resolves IP for either LXC container or QEMU VM.
func (c *Client) ResolveInstanceIP(ctx context.Context, cont LXCContainerSummary, cfg *LXCConfig) (string, string, error) {
	if strings.EqualFold(cont.Type, "qemu") {
		return c.ResolveQemuIP(ctx, cont.Node, cont.VMID, cfg)
	}
	return c.ResolveContainerIP(ctx, cont.Node, cont.VMID, cfg)
}

// ListClusterContainers queries /api2/json/cluster/resources?type=vm for all LXC containers and QEMU VMs across the cluster.
func (c *Client) ListClusterContainers(ctx context.Context) ([]LXCContainerSummary, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/api2/json/cluster/resources?type=vm")
	if err != nil {
		return nil, err
	}

	var res APIResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	var allItems []LXCContainerSummary
	if err := json.Unmarshal(res.Data, &allItems); err != nil {
		return nil, err
	}

	var instances []LXCContainerSummary
	for _, item := range allItems {
		if strings.EqualFold(item.Type, "lxc") || strings.EqualFold(item.Type, "qemu") {
			instances = append(instances, item)
		}
	}
	return instances, nil
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

	onlineNodes, _ := c.GetNodes(ctx)

	// If targetNodes were configured, ensure at least one matches an online node.
	// If none match (e.g. user entered "main" but actual cluster node is "nthms"),
	// fall back to all online nodes rather than returning 0 items.
	if len(targetNodes) > 0 && len(onlineNodes) > 0 {
		hasMatch := false
		for _, tn := range targetNodes {
			for _, on := range onlineNodes {
				if strings.EqualFold(tn, on) {
					hasMatch = true
					break
				}
			}
			if hasMatch {
				break
			}
		}
		if !hasMatch {
			targetNodes = onlineNodes
		}
	}

	var allRoutes []PVERoute
	var allStreams []PVEStream
	var allContainers []LXCContainerSummary

	// Map of container unique key (node:vmid) -> LXCContainerSummary
	seenContainers := make(map[string]LXCContainerSummary)

	// Strategy 1: Check cluster resources endpoint first (covers all nodes even without /nodes permission)
	if clusterContainers, err := c.ListClusterContainers(ctx); err == nil && len(clusterContainers) > 0 {
		for _, cont := range clusterContainers {
			if len(targetNodes) > 0 {
				match := false
				for _, tn := range targetNodes {
					if strings.EqualFold(cont.Node, tn) {
						match = true
						break
					}
				}
				if !match {
					continue
				}
			}
			key := fmt.Sprintf("%s:%d", cont.Node, cont.VMID)
			seenContainers[key] = cont
		}
	}

	// Strategy 2: If no containers discovered via cluster resources, query node-by-node for LXCs and QEMU VMs
	if len(seenContainers) == 0 {
		nodesToQuery := targetNodes
		if len(nodesToQuery) == 0 {
			nodesToQuery = onlineNodes
		}

		for _, nodeName := range nodesToQuery {
			// Query LXC
			if containers, err := c.ListLXCContainers(ctx, nodeName); err == nil {
				for _, cont := range containers {
					key := fmt.Sprintf("%s:%d", cont.Node, cont.VMID)
					seenContainers[key] = cont
				}
			}
			// Query QEMU VMs
			if vms, err := c.ListQemuVMs(ctx, nodeName); err == nil {
				for _, vm := range vms {
					key := fmt.Sprintf("%s:%d", vm.Node, vm.VMID)
					seenContainers[key] = vm
				}
			}
		}
	}

	// Process all collected containers and VMs
	for _, cont := range seenContainers {
		nodeName := cont.Node

		// Only process running instances for route configuration
		if !strings.EqualFold(cont.Status, "running") {
			allContainers = append(allContainers, cont)
			continue
		}

		// Get instance configuration (for Notes and net fallback)
		var cfg *LXCConfig
		if strings.EqualFold(cont.Type, "qemu") {
			cfg, _ = c.GetQemuConfig(ctx, nodeName, cont.VMID)
		} else {
			cfg, _ = c.GetLXCConfig(ctx, nodeName, cont.VMID)
		}

		if cfg != nil {
			if cont.Tags == "" {
				cont.Tags = cfg.Tags
			}
			cont.Notes = cfg.Description
		}

		// Resolve IP (via Guest Agent, Cloud-Init, or interfaces)
		ip, iface, _ := c.ResolveInstanceIP(ctx, cont, cfg)
		if ip != "" {
			cont.IP = ip
		}

		// Parse notes and tags for explicit override host/IP
		notes := cont.Notes
		tags := cont.Tags
		tagMeta := ParsePVETags(tags)
		noteMeta := ParsePVENotes(notes)
		for k, v := range noteMeta {
			tagMeta[k] = v
		}

		if explicitHost, ok := getMetaVal(tagMeta, "host", "forward_host", "forward.host", "ip"); ok && explicitHost != "" {
			ip = explicitHost
			cont.IP = ip
		}

		allContainers = append(allContainers, cont)

		if ip == "" {
			continue
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

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	return allRoutes, allStreams, allContainers, nil
}
