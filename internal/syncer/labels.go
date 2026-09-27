package syncer

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/raddadengineer/npm-autodiscovery/internal/config"
	"github.com/raddadengineer/npm-autodiscovery/internal/docker"
	"github.com/raddadengineer/npm-autodiscovery/internal/middleware"
)

// HealthRoutingConfig defines container healthcheck-aware zero-502 routing settings.
type HealthRoutingConfig struct {
	Enabled        bool          `json:"enabled"`
	FallbackAction string        `json:"fallback_action"` // "disable", "redirect", "keep"
	Timeout        time.Duration `json:"timeout"`         // Default: 60s
	RedirectTarget string        `json:"redirect_target"` // Optional 503 maintenance page or fallback URL
}

// ContainerLocationConfig represents a parsed path route defined on or for a container.
type ContainerLocationConfig struct {
	Path                  string             `json:"path"`
	ForwardScheme         string             `json:"forward_scheme"`
	ForwardHost           string             `json:"forward_host"`
	ForwardPort           int                `json:"forward_port"`
	StripPrefix           bool               `json:"strip_prefix"`
	AllowWebsocketUpgrade bool               `json:"allow_websocket_upgrade"`
	AdvancedConfig        string             `json:"advanced_config"`
	Middlewares           *middleware.Config `json:"middlewares,omitempty"`
}

// ContainerStreamConfig represents parsed Layer 4 TCP/UDP stream discovery settings.
type ContainerStreamConfig struct {
	Enabled        bool   `json:"enabled"`
	ContainerID    string `json:"container_id"`
	ContainerName  string `json:"container_name"`
	IncomingPort   int    `json:"incoming_port"`
	ForwardingHost string `json:"forwarding_host"`
	ForwardingPort int    `json:"forwarding_port"`
	TCPForwarding  bool   `json:"tcp_forwarding"`
	UDPForwarding  bool   `json:"udp_forwarding"`
}

// ContainerUpstreamConfig represents load balancing settings when scaled across multiple replicas.
type ContainerUpstreamConfig struct {
	Algorithm   string `json:"algorithm"`    // "least_conn", "round_robin", "ip_hash"
	MaxFails    int    `json:"max_fails"`    // Default: 3
	FailTimeout string `json:"fail_timeout"` // Default: "10s"
	Name        string `json:"name"`         // Optional custom name
}

// ContainerProxyConfig represents the parsed discovery settings for a container.
type ContainerProxyConfig struct {
	Enabled               bool
	ContainerID           string
	ContainerName         string
	DomainNames           []string
	Path                  string // Primary subpath, defaults to "/"
	StripPrefix           bool
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
	Locations             []ContainerLocationConfig
	Upstream              ContainerUpstreamConfig
	HealthCheck           HealthRoutingConfig
	Middlewares           *middleware.Config
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

	// Parse primary subpath (defaults to "/")
	primaryPath := "/"
	if p, ok := getLabelValue(labels, cfg.LabelPrefix, "frontend.path", "location", "path"); ok {
		p = strings.TrimSpace(p)
		if p != "" {
			if !strings.HasPrefix(p, "/") {
				p = "/" + p
			}
			primaryPath = p
		}
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

	// If root location specifies port: npm.location./.forward_port
	if rootPortVal, ok := getLabelValue(labels, cfg.LabelPrefix, "location./.forward_port", "location./.port"); ok {
		if p, err := strconv.Atoi(strings.TrimSpace(rootPortVal)); err == nil && p > 0 && p <= 65535 {
			port = p
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
	if rootHostVal, ok := getLabelValue(labels, cfg.LabelPrefix, "location./.forward_host"); ok {
		forwardHost = strings.TrimSpace(rootHostVal)
	}

	// Strip prefix on primary path
	stripPrefix := false
	if sp, ok := getLabelValue(labels, cfg.LabelPrefix, "location.strip_prefix", "strip_prefix"); ok {
		stripPrefix = parseBool(sp, false)
	}

	// Custom advanced Nginx configuration
	advancedConfig, _ := getLabelValue(labels, cfg.LabelPrefix, "advanced_config", "nginx_config")

	// Middlewares Engine (Phase 1)
	midCfg, hasMid := middleware.ParseLabels(labels, cfg.LabelPrefix)
	if hasMid && midCfg != nil {
		midDirectives := middleware.GenerateNginxConfig(midCfg)
		if midDirectives != "" {
			if advancedConfig != "" {
				advancedConfig = midDirectives + "\n\n" + advancedConfig
			} else {
				advancedConfig = midDirectives
			}
		}
	}

	// If strip prefix is set and not on root "/", synthesize rewrite directive
	if stripPrefix && primaryPath != "/" {
		cleanPrefix := strings.TrimSuffix(primaryPath, "/")
		rewriteDirective := fmt.Sprintf("rewrite ^%s/?(.*)$ /$1 break;", cleanPrefix)
		if advancedConfig != "" {
			advancedConfig = rewriteDirective + "\n\n" + advancedConfig
		} else {
			advancedConfig = rewriteDirective
		}
	}

	// Parse Custom Locations declared directly via npm.location.<path>.*
	customLocations := parseCustomLocationsFromLabels(labels, cfg.LabelPrefix, port, scheme)

	// Upstream Load Balancing Configuration (Phase 2.3)
	upstreamAlgo := "least_conn"
	if ua, ok := getLabelValue(labels, cfg.LabelPrefix, "upstream.algorithm", "upstream.method", "load_balancer", "lb_method"); ok {
		upstreamAlgo = strings.ToLower(strings.TrimSpace(ua))
	}
	upstreamMaxFails := 3
	if umf, ok := getLabelValue(labels, cfg.LabelPrefix, "upstream.max_fails"); ok {
		if val, err := strconv.Atoi(strings.TrimSpace(umf)); err == nil && val > 0 {
			upstreamMaxFails = val
		}
	}
	upstreamFailTimeout := "10s"
	if uft, ok := getLabelValue(labels, cfg.LabelPrefix, "upstream.fail_timeout"); ok {
		uft = strings.TrimSpace(uft)
		if uft != "" {
			upstreamFailTimeout = uft
		}
	}
	upstreamName, _ := getLabelValue(labels, cfg.LabelPrefix, "upstream.name")

	upstreamCfg := ContainerUpstreamConfig{
		Algorithm:   upstreamAlgo,
		MaxFails:    upstreamMaxFails,
		FailTimeout: upstreamFailTimeout,
		Name:        upstreamName,
	}

	// HealthCheck Routing (Phase 1: Zero-502 HealthCheck-Aware Routing)
	healthEnabled := cfg.DefaultHealthcheckEnabled
	if val, ok := getLabelValue(labels, cfg.LabelPrefix, "healthcheck.enabled", "healthcheck.enable", "healthcheck"); ok {
		healthEnabled = parseBool(val, healthEnabled)
	}

	healthTimeout := cfg.DefaultHealthcheckTimeout
	if val, ok := getLabelValue(labels, cfg.LabelPrefix, "healthcheck.timeout"); ok {
		if dur, err := time.ParseDuration(val); err == nil && dur > 0 {
			healthTimeout = dur
		}
	}

	healthFallback := cfg.DefaultHealthcheckFallback
	if val, ok := getLabelValue(labels, cfg.LabelPrefix, "healthcheck.fallback_action", "healthcheck.fallback"); ok {
		val = strings.ToLower(strings.TrimSpace(val))
		if val == "disable" || val == "redirect" || val == "keep" {
			healthFallback = val
		}
	}

	redirectTarget, _ := getLabelValue(labels, cfg.LabelPrefix, "healthcheck.redirect_target", "healthcheck.redirect")

	healthCfg := HealthRoutingConfig{
		Enabled:        healthEnabled,
		FallbackAction: healthFallback,
		Timeout:        healthTimeout,
		RedirectTarget: redirectTarget,
	}

	return &ContainerProxyConfig{
		Enabled:               true,
		ContainerID:           c.ID,
		ContainerName:         containerName,
		DomainNames:           domainNames,
		Path:                  primaryPath,
		StripPrefix:           stripPrefix,
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
		Locations:             customLocations,
		Upstream:              upstreamCfg,
		HealthCheck:           healthCfg,
		Middlewares:           midCfg,
	}, true, nil
}

// parseCustomLocationsFromLabels extracts subpath location blocks declared on a single container.
func parseCustomLocationsFromLabels(labels map[string]string, prefix string, defaultPort int, defaultScheme string) []ContainerLocationConfig {
	// Look for labels matching (prefix + "location.") or "location."
	// e.g. npm.location./api.forward_port=8080, npm.location./api.strip_prefix=true
	locMap := make(map[string]*ContainerLocationConfig)

	prefixes := []string{prefix + "location.", "location."}

	for k, v := range labels {
		var remainder string
		for _, pfx := range prefixes {
			if strings.HasPrefix(k, pfx) {
				remainder = strings.TrimPrefix(k, pfx)
				break
			}
		}
		if remainder == "" || !strings.HasPrefix(remainder, "/") {
			continue
		}

		lastDot := strings.LastIndex(remainder, ".")
		if lastDot == -1 {
			continue
		}

		locPath := remainder[:lastDot]
		attr := strings.ToLower(remainder[lastDot+1:])

		// Root "/" is handled by primary host properties
		if locPath == "/" {
			continue
		}

		loc, ok := locMap[locPath]
		if !ok {
			loc = &ContainerLocationConfig{
				Path:          locPath,
				ForwardScheme: defaultScheme,
				ForwardPort:   defaultPort,
			}
			locMap[locPath] = loc
		}

		val := strings.TrimSpace(v)
		switch attr {
		case "forward_port", "port":
			if p, err := strconv.Atoi(val); err == nil && p > 0 {
				loc.ForwardPort = p
			}
		case "forward_host", "host", "target_host":
			loc.ForwardHost = val
		case "forward_scheme", "scheme", "protocol":
			if strings.EqualFold(val, "https") {
				loc.ForwardScheme = "https"
			} else {
				loc.ForwardScheme = "http"
			}
		case "strip_prefix":
			loc.StripPrefix = parseBool(val, false)
		case "websocket", "allow_websocket_upgrade":
			loc.AllowWebsocketUpgrade = parseBool(val, false)
		case "advanced_config", "nginx_config":
			loc.AdvancedConfig = val
		}
	}

	var results []ContainerLocationConfig
	for _, loc := range locMap {
		var directives []string
		if loc.StripPrefix {
			cleanPrefix := strings.TrimSuffix(loc.Path, "/")
			if cleanPrefix != "" {
				directives = append(directives, fmt.Sprintf("rewrite ^%s/?(.*)$ /$1 break;", cleanPrefix))
			}
		}
		if loc.AllowWebsocketUpgrade {
			directives = append(directives, "proxy_set_header Upgrade $http_upgrade;\nproxy_set_header Connection \"upgrade\";")
		}
		if len(directives) > 0 {
			combined := strings.Join(directives, "\n\n")
			if loc.AdvancedConfig != "" {
				loc.AdvancedConfig = combined + "\n\n" + loc.AdvancedConfig
			} else {
				loc.AdvancedConfig = combined
			}
		}
		results = append(results, *loc)
	}

	return results
}

// ParseContainerStreams extracts Layer 4 TCP/UDP stream proxy configs from container labels.
func ParseContainerStreams(c *docker.ContainerInspect, cfg *config.Config) ([]ContainerStreamConfig, bool, error) {
	if c == nil {
		return nil, false, fmt.Errorf("container is nil")
	}

	labels := c.Config.Labels
	if labels == nil {
		labels = make(map[string]string)
	}

	containerName := strings.TrimPrefix(c.Name, "/")

	// Check if stream is explicitly enabled or disabled
	if enabledVal, ok := getLabelValue(labels, cfg.LabelPrefix, "stream.enabled", "stream.enable"); ok {
		if strings.EqualFold(enabledVal, "false") || enabledVal == "0" {
			return nil, false, nil
		}
	}

	var streamConfigs []ContainerStreamConfig

	// 1. Primary stream definition: npm.stream.incoming_port or npm.stream.port
	incomingPortStr, hasIncoming := getLabelValue(labels, cfg.LabelPrefix, "stream.incoming_port", "stream.port")
	if hasIncoming && strings.TrimSpace(incomingPortStr) != "" {
		inPort, err := strconv.Atoi(strings.TrimSpace(incomingPortStr))
		if err == nil && inPort > 0 && inPort <= 65535 {
			fwdPort := inPort
			if fwdPortStr, ok := getLabelValue(labels, cfg.LabelPrefix, "stream.forward_port", "stream.forwarding_port"); ok {
				if fp, err := strconv.Atoi(strings.TrimSpace(fwdPortStr)); err == nil && fp > 0 && fp <= 65535 {
					fwdPort = fp
				}
			}
			fwdHost, _ := getLabelValue(labels, cfg.LabelPrefix, "stream.forward_host", "stream.forwarding_host")

			// Protocol parsing: "tcp", "udp", "both"
			proto, hasProto := getLabelValue(labels, cfg.LabelPrefix, "stream.protocol", "stream.proto")
			proto = strings.ToLower(strings.TrimSpace(proto))

			tcpForwarding := true
			udpForwarding := false
			if hasProto {
				switch proto {
				case "udp":
					tcpForwarding = false
					udpForwarding = true
				case "both", "tcp+udp", "all":
					tcpForwarding = true
					udpForwarding = true
				case "tcp":
					tcpForwarding = true
					udpForwarding = false
				}
			}

			// Overrides
			if val, ok := getLabelValue(labels, cfg.LabelPrefix, "stream.tcp_forwarding", "stream.tcp"); ok {
				tcpForwarding = parseBool(val, tcpForwarding)
			}
			if val, ok := getLabelValue(labels, cfg.LabelPrefix, "stream.udp_forwarding", "stream.udp"); ok {
				udpForwarding = parseBool(val, udpForwarding)
			}

			streamConfigs = append(streamConfigs, ContainerStreamConfig{
				Enabled:        true,
				ContainerID:    c.ID,
				ContainerName:  containerName,
				IncomingPort:   inPort,
				ForwardingHost: fwdHost,
				ForwardingPort: fwdPort,
				TCPForwarding:  tcpForwarding,
				UDPForwarding:  udpForwarding,
			})
		}
	}

	// 2. Named stream definitions: npm.stream.<name>.incoming_port
	// e.g. npm.stream.mc.incoming_port=25565
	namedStreams := make(map[string]map[string]string)
	streamPrefix := cfg.LabelPrefix + "stream."
	for k, v := range labels {
		if strings.HasPrefix(k, streamPrefix) {
			rem := strings.TrimPrefix(k, streamPrefix)
			parts := strings.SplitN(rem, ".", 2)
			if len(parts) == 2 {
				name := parts[0]
				field := parts[1]
				if name != "incoming_port" && name != "forward_port" && name != "protocol" && name != "tcp_forwarding" && name != "udp_forwarding" && name != "enabled" {
					if namedStreams[name] == nil {
						namedStreams[name] = make(map[string]string)
					}
					namedStreams[name][field] = v
				}
			}
		}
	}

	for _, nLabels := range namedStreams {
		inPortStr := nLabels["incoming_port"]
		if inPortStr == "" {
			inPortStr = nLabels["port"]
		}
		if inPortStr == "" {
			continue
		}
		inPort, err := strconv.Atoi(strings.TrimSpace(inPortStr))
		if err != nil || inPort <= 0 || inPort > 65535 {
			continue
		}
		fwdPort := inPort
		if fpStr := nLabels["forward_port"]; fpStr != "" {
			if fp, err := strconv.Atoi(strings.TrimSpace(fpStr)); err == nil && fp > 0 && fp <= 65535 {
				fwdPort = fp
			}
		}
		fwdHost := nLabels["forward_host"]
		proto := strings.ToLower(strings.TrimSpace(nLabels["protocol"]))
		tcp := true
		udp := false
		switch proto {
		case "udp":
			tcp = false
			udp = true
		case "both", "tcp+udp", "all":
			tcp = true
			udp = true
		}
		if val, ok := nLabels["tcp_forwarding"]; ok {
			tcp = parseBool(val, tcp)
		}
		if val, ok := nLabels["udp_forwarding"]; ok {
			udp = parseBool(val, udp)
		}

		streamConfigs = append(streamConfigs, ContainerStreamConfig{
			Enabled:        true,
			ContainerID:    c.ID,
			ContainerName:  containerName,
			IncomingPort:   inPort,
			ForwardingHost: fwdHost,
			ForwardingPort: fwdPort,
			TCPForwarding:  tcp,
			UDPForwarding:  udp,
		})
	}

	if len(streamConfigs) == 0 {
		return nil, false, nil
	}

	return streamConfigs, true, nil
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
		// Handle accidental duplicate prefix (e.g. "npm.npm.certificate_id")
		if strings.HasSuffix(prefix, ".") {
			doublePrefixKey := prefix + prefix + key
			if val, exists := labels[doublePrefixKey]; exists {
				return strings.TrimSpace(val), true
			}
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
