package npm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Client is a production-grade HTTP client for the Nginx Proxy Manager official API.
type Client struct {
	baseURL    string
	identity   string
	secret     string
	httpClient *http.Client

	mu        sync.RWMutex
	token     string
	expiresAt time.Time
	connected bool
	lastError string
}

// NewClient creates a new Nginx Proxy Manager API client.
func NewClient(baseURL, identity, secret string, timeout time.Duration) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		identity: identity,
		secret:   secret,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Authenticate forces token retrieval using credentials.
func (c *Client) Authenticate(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.authenticateLocked(ctx)
}

// authenticateLocked performs auth request while mutex is held.
func (c *Client) authenticateLocked(ctx context.Context) error {
	payload := AuthRequest{
		Identity: c.identity,
		Secret:   c.secret,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal auth payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s/api/tokens", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.connected = false
		c.lastError = err.Error()
		return fmt.Errorf("npm auth request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		c.connected = false
		c.lastError = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
		return fmt.Errorf("npm auth returned %d: %s", resp.StatusCode, string(body))
	}

	var authResp TokenResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		c.connected = false
		c.lastError = "invalid auth response format"
		return fmt.Errorf("failed to decode auth response: %w", err)
	}

	c.token = authResp.Token

	// Parse expiration timestamp or default to 24h if missing/unparseable
	if parsedExpires, err := time.Parse(time.RFC3339, authResp.Expires); err == nil {
		c.expiresAt = parsedExpires
	} else {
		c.expiresAt = time.Now().Add(24 * time.Hour)
	}

	c.connected = true
	c.lastError = ""
	return nil
}

// ensureToken ensures a valid JWT token exists and renews it if nearing expiration.
func (c *Client) ensureToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	// Refresh if token is empty or within 5 minutes of expiring
	needsRefresh := c.token == "" || time.Until(c.expiresAt) < 5*time.Minute
	currentTok := c.token
	c.mu.RUnlock()

	if !needsRefresh {
		return currentTok, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double check under write lock
	if c.token != "" && time.Until(c.expiresAt) >= 5*time.Minute {
		return c.token, nil
	}

	if err := c.authenticateLocked(ctx); err != nil {
		return "", err
	}

	return c.token, nil
}

// doRequest performs an authenticated HTTP request, retrying once on 401 Unauthorized.
func (c *Client) doRequest(ctx context.Context, method, path string, bodyData []byte) (*http.Response, error) {
	token, err := c.ensureToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("authentication error: %w", err)
	}

	url := fmt.Sprintf("%s%s", c.baseURL, path)

	makeReq := func(tok string) (*http.Request, error) {
		var body io.Reader
		if bodyData != nil {
			body = bytes.NewReader(bodyData)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tok))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		return req, nil
	}

	req, err := makeReq(token)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.mu.Lock()
		c.connected = false
		c.lastError = err.Error()
		c.mu.Unlock()
		return nil, fmt.Errorf("npm request %s %s failed: %w", method, path, err)
	}

	// If 401 Unauthorized, token may have been revoked or invalidated; re-authenticate and retry once
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		c.mu.Lock()
		authErr := c.authenticateLocked(ctx)
		newTok := c.token
		c.mu.Unlock()

		if authErr != nil {
			return nil, fmt.Errorf("re-authentication failed after 401: %w", authErr)
		}

		req, err = makeReq(newTok)
		if err != nil {
			return nil, err
		}

		resp, err = c.httpClient.Do(req)
		if err != nil {
			c.mu.Lock()
			c.connected = false
			c.lastError = err.Error()
			c.mu.Unlock()
			return nil, fmt.Errorf("npm retry request %s %s failed: %w", method, path, err)
		}
	}

	c.mu.Lock()
	c.connected = true
	c.lastError = ""
	c.mu.Unlock()

	return resp, nil
}

// GetProxyHosts retrieves all configured proxy hosts from NPM.
func (c *Client) GetProxyHosts(ctx context.Context) ([]ProxyHost, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/nginx/proxy-hosts", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch proxy hosts (%d): %s", resp.StatusCode, string(body))
	}

	var hosts []ProxyHost
	if err := json.NewDecoder(resp.Body).Decode(&hosts); err != nil {
		return nil, fmt.Errorf("failed to decode proxy hosts: %w", err)
	}

	return hosts, nil
}

// CreateProxyHost creates a new proxy host record in NPM.
func (c *Client) CreateProxyHost(ctx context.Context, hostReq *ProxyHostRequest) (*ProxyHost, error) {
	data, err := json.Marshal(hostReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal create host payload: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/api/nginx/proxy-hosts", data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create proxy host failed (%d): %s", resp.StatusCode, string(body))
	}

	var created ProxyHost
	if err := json.Unmarshal(body, &created); err != nil {
		return nil, fmt.Errorf("failed to decode created proxy host: %w", err)
	}

	return &created, nil
}

// UpdateProxyHost updates an existing proxy host record in NPM.
func (c *Client) UpdateProxyHost(ctx context.Context, id int, hostReq *ProxyHostRequest) (*ProxyHost, error) {
	data, err := json.Marshal(hostReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal update host payload: %w", err)
	}

	path := fmt.Sprintf("/api/nginx/proxy-hosts/%d", id)
	resp, err := c.doRequest(ctx, http.MethodPut, path, data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update proxy host %d failed (%d): %s", id, resp.StatusCode, string(body))
	}

	var updated ProxyHost
	if err := json.Unmarshal(body, &updated); err != nil {
		return nil, fmt.Errorf("failed to decode updated proxy host: %w", err)
	}

	return &updated, nil
}

// DeleteProxyHost removes a proxy host record from NPM.
func (c *Client) DeleteProxyHost(ctx context.Context, id int) error {
	path := fmt.Sprintf("/api/nginx/proxy-hosts/%d", id)
	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete proxy host %d failed (%d): %s", id, resp.StatusCode, string(body))
	}

	return nil
}

// Status returns current connection status and health info.
func (c *Client) Status() (connected bool, lastError string, baseURL string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected, c.lastError, c.baseURL
}
