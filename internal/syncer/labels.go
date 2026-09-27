package syncer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sampson/npm-autodiscovery/internal/config"
	"github.com/sampson/npm-autodiscovery/internal/docker"
)

// ContainerProxyConfig represents the parsed discovery settings for a container.
type ContainerProxyConfig struct {
	Enabled               bool
	ContainerID           string
	ContainerName         string
	DomainNames           []string
	ForwardScheme         string
	ForwardHost           string
	ForwardPort           int
	SSLEnabled            bool
	SSLForced             bool
	CertificateID         interface{}
	AllowWebsocketUpgrade bool
	BlockExploits         bool
	CachingEnabled        bool
	HTTP2Support          bool
	HSTSEnabled           bool
	HSTSSubdomains        bool
	AccessListID          string
	AdvancedConfig        string
}

// ParseContainerLabels extracts NPM proxy configuration from container labels and ports.
func ParseContainerLabels(c *docker.ContainerInspect, cfg *config.Config) (*ContainerProxyConfig, bool, error) {
	if c == nil {
		return nil, false, fmt.Errorf("container is nil")
	}

	labels := c.Config.Labels
	if labels == nil {
		labels = make(map[string]string)
	}

	containerName := strings.TrimPrefix(c.Name, "/")

	// Check if explicitly disabled or enabled
	enabledStr, hasEnabled := getLabelValue(labels, cfg.LabelPrefix, "enable", "enabled")
	if hasEnabled && (strings.EqualFold(enabledStr, "false") || enabledStr == "0") {
		return nil, false, nil
	}

	// Lookup domain labels: supports npm.frontend.domain, npm.domain, npm.domains
	domainStr, hasDomain := getLabelValue(labels, cfg.LabelPrefix, "frontend.domain", "domain", "domains", "frontend.rule")
	if !hasDomain || strings.TrimSpace(domainStr) == "" {
		// Not configured for auto-discovery
		return nil, false, nil
	}

	// Parse comma-separated or space-separated domain names
	rawDomains := strings.FieldsFunc(domainStr, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';'
	})
	var domainNames []string
	for _, d := range rawDomains {
		clean := strings.TrimSpace(d)
		if clean != "" {
			domainNames = append(domainNames, clean)
		}
	}
	if len(domainNames) == 0 {
		return nil, false, fmt.Errorf("no valid domain names found in label for container %s", containerName)
	}

	// Parse port
	port := 0
	portStr, hasPort := getLabelValue(labels, cfg.LabelPrefix, "frontend.port", "port")
	if hasPort && strings.TrimSpace(portStr) != "" {
		p, err := strconv.Atoi(strings.TrimSpace(portStr))
		if err == nil && p > 0 && p <= 65535 {
			port = p
		} else {
			return nil, false, fmt.Errorf("invalid port value '%s' for container %s", portStr, containerName)
		}
	}

	// If port not specified in labels, inspect exposed ports or network ports
	if port == 0 {
		port = detectExposedPort(c)
	}

	if port == 0 {
		return nil, false, fmt.Errorf("container %s has domains %v but no port specified and no exposed ports detected", containerName, domainNames)
	}

	// Forward Scheme
	scheme, hasScheme := getLabelValue(labels, cfg.LabelPrefix, "forward_scheme", "scheme", "protocol")
	if !hasScheme || strings.TrimSpace(scheme) == "" {
		scheme = cfg.DefaultForwardScheme
	}
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if scheme != "https" {
		scheme = "http"
	}

	// SSL Enabled
	sslEnabled := cfg.DefaultSSLEnabled
	if val, ok := getLabelValue(labels, cfg.LabelPrefix, "ssl.enabled", "ssl", "tls"); ok {
		sslEnabled = parseBool(val, sslEnabled)
	}

	// SSL Forced
	sslForced := cfg.DefaultSSLForced
	if val, ok := getLabelValue(labels, cfg.LabelPrefix, "ssl.forced", "ssl_forced", "force_ssl"); ok {
		sslForced = parseBool(val, sslForced)
	}

	// Certificate ID
	var certID interface{} = 0
	if val, ok := getLabelValue(labels, cfg.LabelPrefix, "certificate_id", "ssl.certificate_id", "cert_id"); ok {
		val = strings.TrimSpace(val)
		if val != "" {
			if idNum, err := strconv.Atoi(val); err == nil {
				certID = idNum
			} else {
				certID = val
			}
		}
	}

	// Websocket support
	websocket := cfg.DefaultWebsocket
	if val, ok := getLabelValue(labels, cfg.LabelPrefix, "websocket", "allow_websocket_upgrade"); ok {
		websocket = parseBool(val, websocket)
	}

	// Block exploits
	blockExploits := cfg.DefaultBlockExploits
	if val, ok := getLabelValue(labels, cfg.LabelPrefix, "block_exploits"); ok {
		blockExploits = parseBool(val, blockExploits)
	}

	// Caching
	caching := false
	if val, ok := getLabelValue(labels, cfg.LabelPrefix, "caching", "caching_enabled"); ok {
		caching = parseBool(val, false)
	}

	// HTTP2 Support
	http2 := false
	if val, ok := getLabelValue(labels, cfg.LabelPrefix, "http2", "http2_support"); ok {
		http2 = parseBool(val, false)
	}

	// HSTS
	hsts := false
	if val, ok := getLabelValue(labels, cfg.LabelPrefix, "hsts", "hsts_enabled"); ok {
		hsts = parseBool(val, false)
	}

	hstsSub := false
	if val, ok := getLabelValue(labels, cfg.LabelPrefix, "hsts_subdomains"); ok {
		hstsSub = parseBool(val, false)
	}

	// Access list ID
	accessListID := "0"
	if val, ok := getLabelValue(labels, cfg.LabelPrefix, "access_list_id", "access_list"); ok {
		accessListID = strings.TrimSpace(val)
	}

	// Custom forward host override
	forwardHost, _ := getLabelValue(labels, cfg.LabelPrefix, "forward_host", "target_host", "target_ip")

	// Custom advanced Nginx configuration
	advancedConfig, _ := getLabelValue(labels, cfg.LabelPrefix, "advanced_config", "nginx_config")

	return &ContainerProxyConfig{
		Enabled:               true,
		ContainerID:           c.ID,
		ContainerName:         containerName,
		DomainNames:           domainNames,
		ForwardScheme:         scheme,
		ForwardHost:           forwardHost,
		ForwardPort:           port,
		SSLEnabled:            sslEnabled,
		SSLForced:             sslForced,
		CertificateID:         certID,
		AllowWebsocketUpgrade: websocket,
		BlockExploits:         blockExploits,
		CachingEnabled:        caching,
		HTTP2Support:          http2,
		HSTSEnabled:           hsts,
		HSTSSubdomains:        hstsSub,
		AccessListID:          accessListID,
		AdvancedConfig:        advancedConfig,
	}, true, nil
}

// getLabelValue retrieves a label value testing multiple key names and prefixes.
func getLabelValue(labels map[string]string, prefix string, keys ...string) (string, bool) {
	for _, key := range keys {
		// Test with configured prefix: e.g. "npm.frontend.domain"
		fullKey := prefix + key
		if val, exists := labels[fullKey]; exists {
			return strings.TrimSpace(val), true
		}
		// Test without prefix if prefix is custom
		if val, exists := labels[key]; exists {
			return strings.TrimSpace(val), true
		}
	}
	return "", false
}

// detectExposedPort checks container exposed ports and returns the first plausible port.
func detectExposedPort(c *docker.ContainerInspect) int {
	if c.Config.ExposedPorts != nil {
		for portProto := range c.Config.ExposedPorts {
			// Format is usually "80/tcp" or "8080/tcp"
			parts := strings.Split(portProto, "/")
			if len(parts) > 0 {
				if p, err := strconv.Atoi(parts[0]); err == nil && p > 0 {
					return p
				}
			}
		}
	}
	return 0
}

func parseBool(str string, fallback bool) bool {
	str = strings.ToLower(strings.TrimSpace(str))
	if str == "true" || str == "1" || str == "yes" || str == "on" {
		return true
	}
	if str == "false" || str == "0" || str == "no" || str == "off" {
		return false
	}
	return fallback
}
