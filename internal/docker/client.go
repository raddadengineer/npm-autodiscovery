package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client is a lightweight Docker Engine API client communicating via Unix socket or TCP.
type Client struct {
	httpClient *http.Client
	socketPath string
	isTCP      bool
	tcpAddress string
	mu         sync.RWMutex
	lastPing   time.Time
	connected  bool
}

// NewClient initializes a new Docker API client.
func NewClient(socketPath string, timeout time.Duration) (*Client, error) {
	if socketPath == "" {
		socketPath = "/var/run/docker.sock"
	}

	cleanPath := socketPath
	isTCP := false
	tcpAddress := ""

	var transport *http.Transport

	if strings.HasPrefix(socketPath, "tcp://") || strings.HasPrefix(socketPath, "http://") {
		isTCP = true
		u, err := url.Parse(socketPath)
		if err != nil {
			return nil, fmt.Errorf("invalid docker tcp address: %w", err)
		}
		tcpAddress = u.Host
		transport = &http.Transport{
			MaxIdleConns:        10,
			IdleConnTimeout:     30 * time.Second,
			DisableCompression: true,
		}
	} else {
		// Unix domain socket
		cleanPath = strings.TrimPrefix(socketPath, "unix://")
		transport = &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: timeout}).DialContext(ctx, "unix", cleanPath)
			},
			DisableCompression: true,
		}
	}

	client := &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
		socketPath: cleanPath,
		isTCP:      isTCP,
		tcpAddress: tcpAddress,
	}

	return client, nil
}

// buildURL constructs an internal HTTP URL for the Docker Engine API request.
func (c *Client) buildURL(endpoint string) string {
	if c.isTCP {
		return fmt.Sprintf("http://%s%s", c.tcpAddress, endpoint)
	}
	// For Unix sockets, host header can be arbitrary (e.g., "docker")
	return fmt.Sprintf("http://docker%s", endpoint)
}

// Ping checks whether the Docker daemon is responding.
func (c *Client) Ping(ctx context.Context) error {
	reqURL := c.buildURL("/_ping")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.connected = false
		return fmt.Errorf("docker socket connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.connected = false
		return fmt.Errorf("docker ping returned unexpected status: %s", resp.Status)
	}

	c.connected = true
	c.lastPing = time.Now()
	return nil
}

// IsConnected returns whether the client is currently connected.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// SocketPath returns the configured socket or connection endpoint.
func (c *Client) SocketPath() string {
	return c.socketPath
}

// GetVersion queries the Docker Engine version details.
func (c *Client) GetVersion(ctx context.Context) (*VersionResponse, error) {
	reqURL := c.buildURL("/version")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("version request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var version VersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return nil, fmt.Errorf("failed to decode version response: %w", err)
	}

	return &version, nil
}

// ListContainers retrieves running (or all) containers from the Docker daemon.
func (c *Client) ListContainers(ctx context.Context, all bool) ([]ContainerSummary, error) {
	endpoint := "/containers/json"
	if all {
		endpoint += "?all=1"
	}
	reqURL := c.buildURL(endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list containers failed (%d): %s", resp.StatusCode, string(body))
	}

	var containers []ContainerSummary
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("failed to decode containers list: %w", err)
	}

	c.mu.Lock()
	c.connected = true
	c.lastPing = time.Now()
	c.mu.Unlock()

	return containers, nil
}

// InspectContainer fetches complete details for a specific container by ID or Name.
func (c *Client) InspectContainer(ctx context.Context, containerID string) (*ContainerInspect, error) {
	reqURL := c.buildURL(fmt.Sprintf("/containers/%s/json", url.PathEscape(containerID)))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("inspect container %s failed: %w", containerID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("inspect container returned status %d: %s", resp.StatusCode, string(body))
	}

	var inspect ContainerInspect
	if err := json.NewDecoder(resp.Body).Decode(&inspect); err != nil {
		return nil, fmt.Errorf("failed to decode inspect response: %w", err)
	}

	return &inspect, nil
}

// StreamEvents starts a long-lived HTTP connection to receive Docker events for containers.
// It delivers events to the returned channel and errors to the error channel.
func (c *Client) StreamEvents(ctx context.Context) (<-chan Event, <-chan error) {
	eventsChan := make(chan Event, 50)
	errorsChan := make(chan error, 10)

	go func() {
		defer close(eventsChan)
		defer close(errorsChan)

		// Filter for container events only
		filterQuery := url.QueryEscape(`{"type":["container"]}`)
		endpoint := fmt.Sprintf("/events?filters=%s", filterQuery)
		reqURL := c.buildURL(endpoint)

		// A dedicated client with no timeout for long-lived streaming
		streamClient := &http.Client{
			Transport: c.httpClient.Transport,
			Timeout:   0,
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
			if err != nil {
				errorsChan <- fmt.Errorf("failed to construct events request: %w", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
					continue
				}
			}

			resp, err := streamClient.Do(req)
			if err != nil {
				c.mu.Lock()
				c.connected = false
				c.mu.Unlock()
				errorsChan <- fmt.Errorf("docker event stream disconnected: %w", err)

				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
					continue
				}
			}

			c.mu.Lock()
			c.connected = true
			c.mu.Unlock()

			reader := bufio.NewReader(resp.Body)
			for {
				line, readErr := reader.ReadBytes('\n')
				if readErr != nil {
					resp.Body.Close()
					if ctx.Err() != nil {
						return
					}
					errorsChan <- fmt.Errorf("reading event stream: %w", readErr)
					break
				}

				lineStr := strings.TrimSpace(string(line))
				if lineStr == "" {
					continue
				}

				var event Event
				if err := json.Unmarshal([]byte(lineStr), &event); err != nil {
					continue
				}

				select {
				case <-ctx.Done():
					resp.Body.Close()
					return
				case eventsChan <- event:
				}
			}

			// Backoff before reconnecting
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}()

	return eventsChan, errorsChan
}
