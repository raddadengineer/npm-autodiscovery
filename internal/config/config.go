package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration options for NPM Auto-Discovery.
type Config struct {
	// NPM API Configuration
	NPMURL      string        `json:"npm_url"`
	NPMUser     string        `json:"npm_user"`
	NPMPass     string        `json:"-"` // Redacted from JSON output for security
	NPMTimeout  time.Duration `json:"npm_timeout"`

	// Docker Socket Configuration
	DockerSocket  string        `json:"docker_socket"`
	DockerTimeout time.Duration `json:"docker_timeout"`

	// Synchronization Settings
	PollInterval time.Duration `json:"poll_interval"`
	NPMNetwork   string        `json:"npm_network"`

	// Host Resolution Strategy: "auto", "name", "ip"
	// - "auto": uses container name if in NPMNetwork or user-defined bridge; falls back to IP
	// - "name": always uses container name (requires NPM and containers to share DNS/network)
	// - "ip":   always uses container IP on the shared network or primary bridge
	ForwardHostStrategy string `json:"forward_host_strategy"`

	// Defaults for Proxy Hosts
	DefaultForwardScheme string `json:"default_forward_scheme"`
	DefaultSSLEnabled    bool   `json:"default_ssl_enabled"`
	DefaultSSLForced     bool   `json:"default_ssl_forced"`
	DefaultWebsocket     bool   `json:"default_websocket"`
	DefaultBlockExploits bool   `json:"default_block_exploits"`

	// Multi-Host / Cluster Configuration
	HostID      string `json:"host_id"`       // Unique identifier for this host (e.g., "worker-01", "vps-east")
	HostIP      string `json:"host_ip"`       // Public/LAN IP or hostname of this Docker host (used for cross-host routing)
	UseHostPort bool   `json:"use_host_port"` // Prefer published host port over container port when routing across hosts

	// Label Configuration
	LabelPrefix string `json:"label_prefix"`

	// Server Settings
	Port     int    `json:"port"`
	LogLevel string `json:"log_level"`
}

// LoadFromEnv loads configuration from environment variables with sensible defaults.
func LoadFromEnv() (*Config, error) {
	npmURL := getEnv("NPM_URL", "http://127.0.0.1:81")
	// Ensure NPMURL doesn't end with a trailing slash
	npmURL = strings.TrimRight(npmURL, "/")

	npmUser := getEnv("NPM_USER", "admin@example.com")
	npmPass := getEnv("NPM_PASS", "changeme")

	// Docker socket path resolution: checks standard linux socket, then macOS socket if standard doesn't exist
	defaultSocket := "/var/run/docker.sock"
	if _, err := os.Stat(defaultSocket); os.IsNotExist(err) {
		homeDir, errHome := os.UserHomeDir()
		if errHome == nil {
			macSocket := homeDir + "/.docker/run/docker.sock"
			if _, errMac := os.Stat(macSocket); errMac == nil {
				defaultSocket = macSocket
			}
		}
	}
	dockerSocket := getEnv("DOCKER_SOCKET", defaultSocket)

	pollIntervalStr := getEnv("POLL_INTERVAL", "30s")
	pollInterval, err := time.ParseDuration(pollIntervalStr)
	if err != nil {
		pollInterval = 30 * time.Second
	}

	npmTimeoutStr := getEnv("NPM_TIMEOUT", "15s")
	npmTimeout, err := time.ParseDuration(npmTimeoutStr)
	if err != nil {
		npmTimeout = 15 * time.Second
	}

	dockerTimeoutStr := getEnv("DOCKER_TIMEOUT", "10s")
	dockerTimeout, err := time.ParseDuration(dockerTimeoutStr)
	if err != nil {
		dockerTimeout = 10 * time.Second
	}

	portStr := getEnv("PORT", "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		port = 8080
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "default-node"
	}
	hostID := getEnv("HOST_ID", getEnv("NODE_ID", hostname))
	hostIP := getEnv("HOST_IP", getEnv("NODE_IP", ""))
	useHostPort := getEnvBool("USE_HOST_PORT", hostIP != "")

	cfg := &Config{
		NPMURL:               npmURL,
		NPMUser:              npmUser,
		NPMPass:              npmPass,
		NPMTimeout:           npmTimeout,
		DockerSocket:         dockerSocket,
		DockerTimeout:        dockerTimeout,
		PollInterval:         pollInterval,
		NPMNetwork:           getEnv("NPM_NETWORK", ""),
		ForwardHostStrategy:  strings.ToLower(getEnv("FORWARD_HOST_STRATEGY", "auto")),
		DefaultForwardScheme: strings.ToLower(getEnv("DEFAULT_FORWARD_SCHEME", "http")),
		DefaultSSLEnabled:    getEnvBool("DEFAULT_SSL_ENABLED", false),
		DefaultSSLForced:     getEnvBool("DEFAULT_SSL_FORCED", false),
		DefaultWebsocket:     getEnvBool("DEFAULT_WEBSOCKET", true),
		DefaultBlockExploits: getEnvBool("DEFAULT_BLOCK_EXPLOITS", true),
		HostID:               hostID,
		HostIP:               hostIP,
		UseHostPort:          useHostPort,
		LabelPrefix:          getEnv("LABEL_PREFIX", "npm."),
		Port:                 port,
		LogLevel:             strings.ToLower(getEnv("LOG_LEVEL", "info")),
	}

	// Basic validation
	if cfg.NPMURL == "" {
		return nil, fmt.Errorf("NPM_URL must not be empty")
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		val = strings.ToLower(strings.TrimSpace(val))
		return val == "true" || val == "1" || val == "yes" || val == "on"
	}
	return defaultVal
}
