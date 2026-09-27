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
	AutoCertificate      bool   `json:"auto_certificate"` // Auto-detect and match NPM SSL certificates based on domain
	AutoSSLForced        bool   `json:"auto_ssl_forced"`  // Force HTTPS when a matching certificate is auto-detected

	// HealthCheck Routing Defaults (Phase 1)
	DefaultHealthcheckEnabled  bool          `json:"default_healthcheck_enabled"`
	DefaultHealthcheckTimeout  time.Duration `json:"default_healthcheck_timeout"`
	DefaultHealthcheckFallback string        `json:"default_healthcheck_fallback"` // "disable", "redirect", "keep"

	// Multi-Host / Cluster Configuration
	HostID      string `json:"host_id"`       // Unique identifier for this host (e.g., "worker-01", "vps-east")
	HostIP      string `json:"host_ip"`       // Public/LAN IP or hostname of this Docker host (used for cross-host routing)
	UseHostPort bool   `json:"use_host_port"` // Prefer published host port over container port when routing across hosts

	// Infrastructure-as-Code (IaC) / Static File Provider
	RoutesFile string `json:"routes_file"` // Path to a declarative static routes YAML/JSON file
	RoutesDir  string `json:"routes_dir"`  // Path to a directory containing declarative routes YAML/JSON files

	// Proxmox VE (PVE) LXC Discovery (Phase 3.1)
	PVEEnabled            bool          `json:"pve_enabled"`
	PVEURL                string        `json:"pve_url"`
	PVETokenID            string        `json:"pve_token_id"`
	PVETokenSecret        string        `json:"-"`
	PVEVerifySSL          bool          `json:"pve_verify_ssl"`
	PVENode               string        `json:"pve_node"`
	PVEPollInterval       time.Duration `json:"pve_poll_interval"`
	PVEPreferredInterface string        `json:"pve_preferred_interface"`
	PVEAllowedSubnets     string        `json:"pve_allowed_subnets"`

	// Canonical LXD & Incus Discovery (Phase 3.2)
	LXDEnabled            bool          `json:"lxd_enabled"`
	LXDSocket             string        `json:"lxd_socket"`
	LXDPreferredInterface string        `json:"lxd_preferred_interface"`
	LXDAllowedSubnets     string        `json:"lxd_allowed_subnets"`
	LXDPollInterval       time.Duration `json:"lxd_poll_interval"`

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

	defaultHealthcheckTimeoutStr := getEnv("DEFAULT_HEALTHCHECK_TIMEOUT", "60s")
	defaultHealthcheckTimeout, err := time.ParseDuration(defaultHealthcheckTimeoutStr)
	if err != nil {
		defaultHealthcheckTimeout = 60 * time.Second
	}
	defaultHealthcheckFallback := strings.ToLower(getEnv("DEFAULT_HEALTHCHECK_FALLBACK", "disable"))
	defaultHealthcheckEnabled := getEnvBool("DEFAULT_HEALTHCHECK_ENABLED", false)

	// Proxmox VE settings
	pveEnabled := getEnvBool("PVE_ENABLED", false)
	pveURL := getEnv("PVE_URL", "")
	pveTokenID := getEnv("PVE_TOKEN_ID", "")
	pveTokenSecret := getEnv("PVE_TOKEN_SECRET", "")
	pveVerifySSL := getEnvBool("PVE_VERIFY_SSL", false)
	pveNode := getEnv("PVE_NODE", "")
	pvePollIntervalStr := getEnv("PVE_POLL_INTERVAL", "15s")
	pvePollInterval, err := time.ParseDuration(pvePollIntervalStr)
	if err != nil {
		pvePollInterval = 15 * time.Second
	}
	pvePreferredInterface := getEnv("PVE_PREFERRED_INTERFACE", "eth0")
	pveAllowedSubnets := getEnv("PVE_ALLOWED_SUBNETS", "")

	// Canonical LXD / Incus settings
	lxdEnabled := getEnvBool("LXD_ENABLED", false)
	defaultLXDSocket := "/var/snap/lxd/common/lxd/unix.socket"
	if _, err := os.Stat(defaultLXDSocket); os.IsNotExist(err) {
		if _, errIncus := os.Stat("/var/lib/incus/unix.socket"); errIncus == nil {
			defaultLXDSocket = "/var/lib/incus/unix.socket"
		} else if _, errApt := os.Stat("/var/lib/lxd/unix.socket"); errApt == nil {
			defaultLXDSocket = "/var/lib/lxd/unix.socket"
		}
	}
	lxdSocket := getEnv("LXD_SOCKET", defaultLXDSocket)
	lxdPreferredInterface := getEnv("LXD_PREFERRED_INTERFACE", "eth0")
	lxdAllowedSubnets := getEnv("LXD_ALLOWED_SUBNETS", "")
	lxdPollIntervalStr := getEnv("LXD_POLL_INTERVAL", "15s")
	lxdPollInterval, err := time.ParseDuration(lxdPollIntervalStr)
	if err != nil {
		lxdPollInterval = 15 * time.Second
	}

	cfg := &Config{
		NPMURL:                     npmURL,
		NPMUser:                    npmUser,
		NPMPass:                    npmPass,
		NPMTimeout:                 npmTimeout,
		DockerSocket:               dockerSocket,
		DockerTimeout:              dockerTimeout,
		PollInterval:               pollInterval,
		NPMNetwork:                 getEnv("NPM_NETWORK", ""),
		ForwardHostStrategy:        strings.ToLower(getEnv("FORWARD_HOST_STRATEGY", "auto")),
		DefaultForwardScheme:       strings.ToLower(getEnv("DEFAULT_FORWARD_SCHEME", "http")),
		DefaultSSLEnabled:          getEnvBool("DEFAULT_SSL_ENABLED", false),
		DefaultSSLForced:           getEnvBool("DEFAULT_SSL_FORCED", false),
		DefaultWebsocket:           getEnvBool("DEFAULT_WEBSOCKET", true),
		DefaultBlockExploits:       getEnvBool("DEFAULT_BLOCK_EXPLOITS", true),
		AutoCertificate:            getEnvBool("AUTO_CERTIFICATE", getEnvBool("AUTO_SSL", true)),
		AutoSSLForced:              getEnvBool("AUTO_SSL_FORCED", true),
		DefaultHealthcheckEnabled:  defaultHealthcheckEnabled,
		DefaultHealthcheckTimeout:  defaultHealthcheckTimeout,
		DefaultHealthcheckFallback: defaultHealthcheckFallback,
		HostID:                     hostID,
		HostIP:                     hostIP,
		UseHostPort:                useHostPort,
		RoutesFile:                 getEnv("ROUTES_FILE", getEnv("CONFIG_FILE", "")),
		RoutesDir:                  getEnv("ROUTES_DIR", getEnv("CONFIG_DIR", "")),
		PVEEnabled:                 pveEnabled,
		PVEURL:                     pveURL,
		PVETokenID:                 pveTokenID,
		PVETokenSecret:             pveTokenSecret,
		PVEVerifySSL:               pveVerifySSL,
		PVENode:                    pveNode,
		PVEPollInterval:            pvePollInterval,
		PVEPreferredInterface:      pvePreferredInterface,
		PVEAllowedSubnets:          pveAllowedSubnets,
		LXDEnabled:                 lxdEnabled,
		LXDSocket:                  lxdSocket,
		LXDPreferredInterface:      lxdPreferredInterface,
		LXDAllowedSubnets:          lxdAllowedSubnets,
		LXDPollInterval:            lxdPollInterval,
		LabelPrefix:                getEnv("LABEL_PREFIX", "npm."),
		Port:                       port,
		LogLevel:                   strings.ToLower(getEnv("LOG_LEVEL", "info")),
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
