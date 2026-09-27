package lxd

import (
	"strconv"
	"strings"
	"time"
)

// ExtractRouteAndStreams extracts reverse proxy routes and stream proxies from instance configuration.
func ExtractRouteAndStreams(
	inst *Instance,
	ip string,
	iface string,
	defaultScheme string,
	defaultSSL bool,
	defaultWebsocket bool,
	defaultBlockExploits bool,
) (*LXDRoute, []LXDStream, bool) {
	if inst == nil || ip == "" {
		return nil, nil, false
	}

	cfg := inst.Config
	if len(cfg) == 0 {
		cfg = inst.ExpandedConfig
	}
	if len(cfg) == 0 {
		return nil, nil, false
	}

	// Normalize user metadata: remove "user." prefix if present, normalize separators
	normalized := make(map[string]string)
	for k, v := range cfg {
		cleanK := strings.TrimPrefix(k, "user.")
		cleanK = strings.TrimPrefix(cleanK, "npm.")
		cleanK = strings.ReplaceAll(cleanK, "-", "_")
		normalized[cleanK] = strings.TrimSpace(v)
	}

	// Check if explicitly enabled/disabled
	if val, ok := getNormalizedVal(normalized, "enable", "enabled"); ok {
		if strings.EqualFold(val, "false") || val == "0" {
			return nil, nil, false
		}
	}

	// 1. Layer 4 Streams Discovery
	var streams []LXDStream
	streamPortStr, hasStreamPort := getNormalizedVal(normalized, "stream.incoming_port", "stream.port", "stream_incoming_port", "stream_port")
	if hasStreamPort {
		if inPort, err := strconv.Atoi(streamPortStr); err == nil && inPort > 0 {
			fPort := inPort
			if fPortStr, ok := getNormalizedVal(normalized, "stream.forward_port", "stream_forward_port"); ok {
				if parsed, errP := strconv.Atoi(fPortStr); errP == nil && parsed > 0 {
					fPort = parsed
				}
			}

			tcp := true
			if tcpStr, ok := getNormalizedVal(normalized, "stream.tcp", "stream_tcp", "stream.tcp_forwarding", "stream_tcp_forwarding"); ok {
				tcp = parseBool(tcpStr, true)
			}
			udp := false
			if udpStr, ok := getNormalizedVal(normalized, "stream.udp", "stream_udp", "stream.udp_forwarding", "stream_udp_forwarding"); ok {
				udp = parseBool(udpStr, false)
			}

			streams = append(streams, LXDStream{
				Name:           inst.Name,
				IncomingPort:   inPort,
				ForwardingHost: ip,
				ForwardingPort: fPort,
				TCPForwarding:  tcp,
				UDPForwarding:  udp,
				Interface:      iface,
			})
		}
	}

	// 2. HTTP Reverse Proxy Discovery
	domainStr, hasDomain := getNormalizedVal(normalized, "domain", "domains", "frontend.domain", "frontend_domain")
	if !hasDomain || strings.TrimSpace(domainStr) == "" {
		if len(streams) > 0 {
			return nil, streams, true
		}
		return nil, nil, false
	}

	var domains []string
	for _, d := range strings.Split(domainStr, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			domains = append(domains, d)
		}
	}
	if len(domains) == 0 {
		if len(streams) > 0 {
			return nil, streams, true
		}
		return nil, nil, false
	}

	port := 80
	if portStr, ok := getNormalizedVal(normalized, "port", "frontend.port", "frontend_port"); ok {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			port = p
		}
	}

	scheme := defaultScheme
	if scheme == "" {
		scheme = "http"
	}
	if sVal, ok := getNormalizedVal(normalized, "scheme", "forward_scheme", "frontend.scheme"); ok && sVal != "" {
		scheme = strings.ToLower(sVal)
	}

	path := "/"
	if pVal, ok := getNormalizedVal(normalized, "path", "frontend.path", "frontend_path"); ok && pVal != "" {
		path = pVal
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
	}

	sslEnabled := defaultSSL
	if sVal, ok := getNormalizedVal(normalized, "ssl", "ssl_enabled"); ok {
		sslEnabled = parseBool(sVal, defaultSSL)
	}

	sslForced := false
	if sVal, ok := getNormalizedVal(normalized, "ssl_forced", "force_ssl"); ok {
		sslForced = parseBool(sVal, false)
	}

	websocket := defaultWebsocket
	if wVal, ok := getNormalizedVal(normalized, "websocket", "allow_websocket_upgrade"); ok {
		websocket = parseBool(wVal, defaultWebsocket)
	}

	blockExploits := defaultBlockExploits
	if bVal, ok := getNormalizedVal(normalized, "block_exploits"); ok {
		blockExploits = parseBool(bVal, defaultBlockExploits)
	}

	caching := false
	if cVal, ok := getNormalizedVal(normalized, "caching", "caching_enabled"); ok {
		caching = parseBool(cVal, false)
	}

	http2 := true
	if hVal, ok := getNormalizedVal(normalized, "http2", "http2_support"); ok {
		http2 = parseBool(hVal, true)
	}

	hsts := false
	if hVal, ok := getNormalizedVal(normalized, "hsts", "hsts_enabled"); ok {
		hsts = parseBool(hVal, false)
	}

	hstsSub := false
	if hsVal, ok := getNormalizedVal(normalized, "hsts_subdomains"); ok {
		hstsSub = parseBool(hsVal, false)
	}

	accessListID, _ := getNormalizedVal(normalized, "access_list_id", "access_list")
	advConfig, _ := getNormalizedVal(normalized, "advanced_config", "custom_config")

	route := &LXDRoute{
		Name:                  inst.Name,
		DomainNames:           domains,
		Path:                  path,
		ForwardScheme:         scheme,
		ForwardHost:           ip,
		ForwardPort:           port,
		Interface:             iface,
		SSLEnabled:            sslEnabled,
		SSLForced:             sslForced,
		AllowWebsocketUpgrade: websocket,
		BlockExploits:         blockExploits,
		CachingEnabled:        caching,
		HTTP2Support:          http2,
		HSTSEnabled:           hsts,
		HSTSSubdomains:        hstsSub,
		AccessListID:          accessListID,
		AdvancedConfig:        advConfig,
		DiscoveredAt:          time.Now(),
	}

	return route, streams, true
}

func getNormalizedVal(m map[string]string, keys ...string) (string, bool) {
	for _, k := range keys {
		if val, ok := m[k]; ok {
			return val, true
		}
		// Try replacing dots with underscores
		if val, ok := m[strings.ReplaceAll(k, ".", "_")]; ok {
			return val, true
		}
	}
	return "", false
}

func parseBool(str string, def bool) bool {
	str = strings.ToLower(strings.TrimSpace(str))
	if str == "true" || str == "1" || str == "yes" || str == "on" {
		return true
	}
	if str == "false" || str == "0" || str == "no" || str == "off" {
		return false
	}
	return def
}
