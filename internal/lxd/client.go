package lxd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Standard socket paths for Canonical LXD and LinuxContainers Incus
var standardSocketPaths = []string{
	"/var/snap/lxd/common/lxd/unix.socket", // Canonical Snap LXD
	"/var/lib/incus/unix.socket",          // Incus
	"/var/lib/lxd/unix.socket",            // Canonical Apt LXD
}

// Client is a Canonical LXD & Incus API client communicating over Unix socket.
type Client struct {
	socketPath         string
	preferredInterface string
	allowedSubnets     []*net.IPNet
	httpClient         *http.Client

	mu            sync.RWMutex
	connected     bool
	serverType    string // "lxd" or "incus"
	serverVersion string
	lastPing      time.Time
}

// AutoDetectSocket finds the first existing LXD/Incus Unix domain socket.
func AutoDetectSocket() string {
	for _, p := range standardSocketPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return standardSocketPaths[0]
}

// NewClient initializes a new LXD/Incus API client.
func NewClient(
	socketPath string,
	timeout time.Duration,
	preferredInterface string,
	allowedSubnetsStr string,
) (*Client, error) {
	if socketPath == "" {
		socketPath = AutoDetectSocket()
	}

	cleanPath := strings.TrimPrefix(socketPath, "unix://")

	subnets, err := ParseSubnets(allowedSubnetsStr)
	if err != nil {
		return nil, fmt.Errorf("lxd: %w", err)
	}

	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	if preferredInterface == "" {
		preferredInterface = "eth0"
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: timeout}).DialContext(ctx, "unix", cleanPath)
		},
		DisableCompression: true,
	}

	return &Client{
		socketPath:         cleanPath,
		preferredInterface: preferredInterface,
		allowedSubnets:     subnets,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
	}, nil
}

// doRequest performs an HTTP request over the Unix socket.
func (c *Client) doRequest(ctx context.Context, method, endpoint string) ([]byte, error) {
	reqURL := fmt.Sprintf("http://unix%s", endpoint)
	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		return nil, fmt.Errorf("lxd unix socket connection error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lxd reading response failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("lxd api error (status %d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// Ping verifies connectivity to the LXD/Incus daemon and detects the server brand and version.
func (c *Client) Ping(ctx context.Context) error {
	data, err := c.doRequest(ctx, http.MethodGet, "/1.0")
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

	var info ServerInfo
	if err := json.Unmarshal(res.Metadata, &info); err == nil {
		c.serverType = info.Environment.Server
		if c.serverType == "" {
			c.serverType = "lxd"
		}
		c.serverVersion = info.Environment.ServerVersion
	}

	c.connected = true
	return nil
}

// IsConnected returns whether the client is connected.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// ServerType returns "lxd" or "incus".
func (c *Client) ServerType() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.serverType == "" {
		return "lxd"
	}
	return c.serverType
}

// ServerVersion returns the server version string.
func (c *Client) ServerVersion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.serverVersion
}

// SocketPath returns the Unix domain socket path.
func (c *Client) SocketPath() string {
	return c.socketPath
}

// ListInstances queries running containers and VMs with full metadata and runtime state.
func (c *Client) ListInstances(ctx context.Context) ([]Instance, error) {
	// Try /1.0/instances?recursion=2 (includes full state)
	data, err := c.doRequest(ctx, http.MethodGet, "/1.0/instances?recursion=2")
	if err != nil {
		// Fallback for legacy LXD /1.0/containers?recursion=2
		data, err = c.doRequest(ctx, http.MethodGet, "/1.0/containers?recursion=2")
		if err != nil {
			return nil, err
		}
	}

	var res APIResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	var instances []Instance
	if err := json.Unmarshal(res.Metadata, &instances); err != nil {
		return nil, err
	}

	// For instances that may not have state populated in recursion, fetch individual state
	for i := range instances {
		if instances[i].State == nil && strings.EqualFold(instances[i].Status, "running") {
			state, err := c.GetInstanceState(ctx, instances[i].Name)
			if err == nil {
				instances[i].State = state
			}
		}
	}

	return instances, nil
}

// GetInstanceState queries /1.0/instances/{name}/state for network addresses.
func (c *Client) GetInstanceState(ctx context.Context, name string) (*InstanceState, error) {
	endpoint := fmt.Sprintf("/1.0/instances/%s/state", name)
	data, err := c.doRequest(ctx, http.MethodGet, endpoint)
	if err != nil {
		// Fallback to /1.0/containers/{name}/state
		endpoint = fmt.Sprintf("/1.0/containers/%s/state", name)
		data, err = c.doRequest(ctx, http.MethodGet, endpoint)
		if err != nil {
			return nil, err
		}
	}

	var res APIResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	var state InstanceState
	if err := json.Unmarshal(res.Metadata, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// StreamEvents opens a persistent lifecycle event stream from /1.0/events?type=lifecycle.
func (c *Client) StreamEvents(ctx context.Context) (<-chan LifecycleEvent, <-chan error) {
	eventChan := make(chan LifecycleEvent, 32)
	errChan := make(chan error, 1)

	go func() {
		defer close(eventChan)
		defer close(errChan)

		backoff := 1 * time.Second
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			reqURL := "http://unix/1.0/events?type=lifecycle"
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
			if err != nil {
				errChan <- err
				time.Sleep(backoff)
				continue
			}

			// Stream client with no read timeout
			streamClient := &http.Client{
				Transport: c.httpClient.Transport,
				Timeout:   0,
			}

			resp, err := streamClient.Do(req)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					errChan <- fmt.Errorf("lxd event stream disconnected: %w", err)
					time.Sleep(backoff)
					continue
				}
			}

			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, ":") {
					continue
				}

				// Events arrive as JSON or SSE "data: {...}"
				cleanJSON := line
				if strings.HasPrefix(cleanJSON, "data:") {
					cleanJSON = strings.TrimSpace(strings.TrimPrefix(cleanJSON, "data:"))
				}

				var ev LifecycleEvent
				if err := json.Unmarshal([]byte(cleanJSON), &ev); err == nil && ev.Type != "" {
					select {
					case eventChan <- ev:
					case <-ctx.Done():
						resp.Body.Close()
						return
					}
				}
			}
			resp.Body.Close()
			time.Sleep(backoff)
		}
	}()

	return eventChan, errChan
}

// DiscoverRoutes scans all running instances, evaluates user.npm metadata, and resolves IPs.
func (c *Client) DiscoverRoutes(
	ctx context.Context,
	defaultScheme string,
	defaultSSL bool,
	defaultWebsocket bool,
	defaultBlockExploits bool,
) ([]LXDRoute, []LXDStream, []Instance, error) {
	instances, err := c.ListInstances(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	var allRoutes []LXDRoute
	var allStreams []LXDStream

	for _, inst := range instances {
		if !strings.EqualFold(inst.Status, "running") && inst.StatusCode != 103 {
			continue
		}

		ip, iface, err := ResolveInstanceIP(inst.State, c.preferredInterface, c.allowedSubnets)
		if err != nil || ip == "" {
			continue
		}

		route, streams, ok := ExtractRouteAndStreams(
			&inst,
			ip,
			iface,
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

	return allRoutes, allStreams, instances, nil
}
