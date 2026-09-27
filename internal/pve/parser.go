package pve

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ParsePVETags extracts key-value metadata from Proxmox tags string.
// Supported tag formats:
//   - "npm-domain=adguard.lan;npm-port=80;npm-ssl=true"
//   - "npm-domain=adguard.lan, npm-port=80, npm-ssl=true"
//   - "npm.domain=adguard.lan npm.port=80"
//   - "npm:domain=adguard.lan"
func ParsePVETags(tags string) map[string]string {
	result := make(map[string]string)
	if strings.TrimSpace(tags) == "" {
		return result
	}

	// Normalize separators (; and , to whitespace)
	normalized := strings.ReplaceAll(tags, ";", " ")
	normalized = strings.ReplaceAll(normalized, ",", " ")

	fields := strings.Fields(normalized)
	for _, field := range fields {
		var key, val string
		if strings.Contains(field, "=") {
			parts := strings.SplitN(field, "=", 2)
			key = parts[0]
			val = parts[1]
		} else if strings.Contains(field, ":") {
			parts := strings.SplitN(field, ":", 2)
			key = parts[0]
			val = parts[1]
		} else {
			// Boolean tag like "npm-ssl" or "npm-enabled"
			key = field
			val = "true"
		}

		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		// Normalize key prefixes: "npm-", "npm:", "npm."
		if strings.HasPrefix(key, "npm-") {
			key = "npm." + strings.TrimPrefix(key, "npm-")
			key = strings.ReplaceAll(key, "-", "_")
		} else if strings.HasPrefix(key, "npm:") {
			key = "npm." + strings.TrimPrefix(key, "npm:")
			key = strings.ReplaceAll(key, "-", "_")
		} else if strings.HasPrefix(key, "npm.") {
			key = strings.ReplaceAll(key, "-", "_")
		}

		result[key] = val
	}

	return result
}

// ParsePVENotes extracts key-value configuration from LXC container Notes / description.
// It supports YAML formatting or line-by-line key: value / key=value syntax.
func ParsePVENotes(notes string) map[string]string {
	result := make(map[string]string)
	if strings.TrimSpace(notes) == "" {
		return result
	}

	// Try YAML unmarshaling first
	var yamlMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(notes), &yamlMap); err == nil && len(yamlMap) > 0 {
		flattenYAML("", yamlMap, result)
		if len(result) > 0 {
			return result
		}
	}

	// Fallback to line-by-line parser
	scanner := bufio.NewScanner(strings.NewReader(notes))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var key, val string
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			key = parts[0]
			val = parts[1]
		} else if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key = parts[0]
			val = parts[1]
		} else {
			continue
		}

		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		// Strip surrounding quotes
		val = strings.Trim(val, `"'`)

		if strings.HasPrefix(key, "npm-") {
			key = "npm." + strings.TrimPrefix(key, "npm-")
		} else if strings.HasPrefix(key, "npm:") {
			key = "npm." + strings.TrimPrefix(key, "npm:")
		}
		key = strings.ReplaceAll(key, "-", "_")

		result[key] = val
	}

	return result
}

func flattenYAML(prefix string, m map[string]interface{}, out map[string]string) {
	for k, v := range m {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}
		fullKey = strings.ReplaceAll(fullKey, "-", "_")

		switch val := v.(type) {
		case map[string]interface{}:
			flattenYAML(fullKey, val, out)
		case []interface{}:
			// Convert slice to comma-separated string
			var strItems []string
			for _, item := range val {
				strItems = append(strItems, strings.TrimSpace(fmt.Sprint(item)))
			}
			out[fullKey] = strings.Join(strItems, ",")
		default:
			out[fullKey] = fmt.Sprint(v)
		}
	}
}

// ExtractRouteAndStreams parses metadata from both Channel A (tags) and Channel B (notes),
// merging them with Notes taking precedence, and resolves routes and streams.
func ExtractRouteAndStreams(
	vmid int,
	node string,
	containerName string,
	ip string,
	iface string,
	tags string,
	notes string,
	defaultScheme string,
	defaultSSL bool,
	defaultWebsocket bool,
	defaultBlockExploits bool,
) (*PVERoute, []PVEStream, bool) {
	tagMeta := ParsePVETags(tags)
	noteMeta := ParsePVENotes(notes)

	// Combine metadata; notes take precedence over tags
	merged := make(map[string]string)
	channel := ""
	if len(tagMeta) > 0 {
		channel = "tags"
		for k, v := range tagMeta {
			merged[k] = v
		}
	}
	if len(noteMeta) > 0 {
		if channel == "tags" {
			channel = "both"
		} else {
			channel = "notes"
		}
		for k, v := range noteMeta {
			merged[k] = v
		}
	}

	if len(merged) == 0 {
		return nil, nil, false
	}

	// Check if explicitly disabled
	if val, ok := getMetaVal(merged, "enable", "enabled"); ok {
		if strings.EqualFold(val, "false") || val == "0" {
			return nil, nil, false
		}
	}

	// 1. Check for Layer 4 TCP/UDP Stream configuration
	var streams []PVEStream
	streamPortStr, hasStreamPort := getMetaVal(merged, "stream.incoming_port", "stream.port", "stream_incoming_port", "stream_port")
	if hasStreamPort {
		if inPort, err := strconv.Atoi(streamPortStr); err == nil && inPort > 0 {
			fPort := inPort
			if fPortStr, ok := getMetaVal(merged, "stream.forward_port", "stream_forward_port"); ok {
				if parsed, errP := strconv.Atoi(fPortStr); errP == nil && parsed > 0 {
					fPort = parsed
				}
			}

			tcp := true
			if tcpStr, ok := getMetaVal(merged, "stream.tcp", "stream_tcp", "stream.tcp_forwarding", "stream_tcp_forwarding"); ok {
				tcp = parseBool(tcpStr, true)
			}
			udp := false
			if udpStr, ok := getMetaVal(merged, "stream.udp", "stream_udp", "stream.udp_forwarding", "stream_udp_forwarding"); ok {
				udp = parseBool(udpStr, false)
			}

			streams = append(streams, PVEStream{
				VMID:           vmid,
				Node:           node,
				ContainerName:  containerName,
				IncomingPort:   inPort,
				ForwardingHost: ip,
				ForwardingPort: fPort,
				TCPForwarding:  tcp,
				UDPForwarding:  udp,
				Interface:      iface,
			})
		}
	}

	// 2. Check for HTTP/HTTPS Reverse Proxy configuration
	domainStr, hasDomain := getMetaVal(merged, "domain", "domains", "frontend.domain", "frontend_domain")
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

	// Determine port
	port := 80
	if portStr, ok := getMetaVal(merged, "port", "frontend.port", "frontend_port"); ok {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			port = p
		}
	}

	// Determine scheme
	scheme := defaultScheme
	if scheme == "" {
		scheme = "http"
	}
	if sVal, ok := getMetaVal(merged, "scheme", "forward_scheme", "frontend.scheme"); ok && sVal != "" {
		scheme = strings.ToLower(sVal)
	}

	// Subpath
	path := "/"
	if pVal, ok := getMetaVal(merged, "path", "frontend.path", "frontend_path"); ok && pVal != "" {
		path = pVal
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
	}

	sslEnabled := defaultSSL
	if sVal, ok := getMetaVal(merged, "ssl", "ssl_enabled"); ok {
		sslEnabled = parseBool(sVal, defaultSSL)
	}

	sslForced := false
	if sVal, ok := getMetaVal(merged, "ssl_forced", "force_ssl"); ok {
		sslForced = parseBool(sVal, false)
	}

	websocket := defaultWebsocket
	if wVal, ok := getMetaVal(merged, "websocket", "allow_websocket_upgrade"); ok {
		websocket = parseBool(wVal, defaultWebsocket)
	}

	blockExploits := defaultBlockExploits
	if bVal, ok := getMetaVal(merged, "block_exploits"); ok {
		blockExploits = parseBool(bVal, defaultBlockExploits)
	}

	caching := false
	if cVal, ok := getMetaVal(merged, "caching", "caching_enabled"); ok {
		caching = parseBool(cVal, false)
	}

	http2 := true
	if hVal, ok := getMetaVal(merged, "http2", "http2_support"); ok {
		http2 = parseBool(hVal, true)
	}

	hsts := false
	if hVal, ok := getMetaVal(merged, "hsts", "hsts_enabled"); ok {
		hsts = parseBool(hVal, false)
	}

	hstsSub := false
	if hsVal, ok := getMetaVal(merged, "hsts_subdomains"); ok {
		hstsSub = parseBool(hsVal, false)
	}

	accessListID, _ := getMetaVal(merged, "access_list_id", "access_list")
	advConfig, _ := getMetaVal(merged, "advanced_config", "custom_config")

	route := &PVERoute{
		VMID:                  vmid,
		Node:                  node,
		ContainerName:         containerName,
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
		Tags:                  tags,
		DiscoveryChannel:      channel,
		DiscoveredAt:          time.Now(),
	}

	return route, streams, true
}

func getMetaVal(m map[string]string, keys ...string) (string, bool) {
	for _, k := range keys {
		// check npm.<key>
		if val, ok := m["npm."+k]; ok {
			return val, true
		}
		// check bare <key>
		if val, ok := m[k]; ok {
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
