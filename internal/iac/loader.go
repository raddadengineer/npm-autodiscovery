package iac

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sampson/npm-autodiscovery/internal/middleware"
	"gopkg.in/yaml.v3"
)

// LoadRoutes scans a file or directory path and loads all declared proxy hosts.
func LoadRoutes(fileOrDirPath string) ([]StaticProxyHost, error) {
	if fileOrDirPath == "" {
		return nil, nil
	}

	info, err := os.Stat(fileOrDirPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error reading config path %s: %w", fileOrDirPath, err)
	}

	var allHosts []StaticProxyHost

	if info.IsDir() {
		entries, err := os.ReadDir(fileOrDirPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config directory %s: %w", fileOrDirPath, err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext == ".yaml" || ext == ".yml" || ext == ".json" {
				filePath := filepath.Join(fileOrDirPath, entry.Name())
				hosts, err := parseFile(filePath)
				if err != nil {
					return nil, fmt.Errorf("error in manifest file %s: %w", filePath, err)
				}
				allHosts = append(allHosts, hosts...)
			}
		}
	} else {
		hosts, err := parseFile(fileOrDirPath)
		if err != nil {
			return nil, err
		}
		allHosts = append(allHosts, hosts...)
	}

	return allHosts, nil
}

func parseFile(filePath string) ([]StaticProxyHost, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Try parsing as RouteManifest: { proxy_hosts: [...] }
	var manifest RouteManifest
	if err := yaml.Unmarshal(data, &manifest); err == nil && len(manifest.ProxyHosts) > 0 {
		return sanitizeHosts(manifest.ProxyHosts, filepath.Base(filePath))
	}

	// Try parsing directly as list of hosts: [ { domain_names: [...] }, ... ]
	var hostList []StaticProxyHost
	if err := yaml.Unmarshal(data, &hostList); err == nil && len(hostList) > 0 {
		return sanitizeHosts(hostList, filepath.Base(filePath))
	}

	// Empty or no valid hosts found in file
	return nil, nil
}

func sanitizeHosts(hosts []StaticProxyHost, fileName string) ([]StaticProxyHost, error) {
	var valid []StaticProxyHost
	for i, h := range hosts {
		// Clean and validate domains
		var cleanDomains []string
		for _, d := range h.DomainNames {
			trimmed := strings.TrimSpace(d)
			if trimmed != "" {
				cleanDomains = append(cleanDomains, trimmed)
			}
		}
		if len(cleanDomains) == 0 {
			return nil, fmt.Errorf("item #%d in %s has no domain_names specified", i+1, fileName)
		}
		h.DomainNames = cleanDomains

		// Clean and validate target
		h.ForwardHost = strings.TrimSpace(h.ForwardHost)
		if h.ForwardHost == "" {
			return nil, fmt.Errorf("item #%d (%v) in %s has no forward_host specified", i+1, h.DomainNames, fileName)
		}

		if h.ForwardPort <= 0 || h.ForwardPort > 65535 {
			return nil, fmt.Errorf("item #%d (%v) in %s has invalid forward_port %d", i+1, h.DomainNames, fileName, h.ForwardPort)
		}

		// Defaults
		scheme := strings.ToLower(strings.TrimSpace(h.ForwardScheme))
		if scheme != "https" {
			scheme = "http"
		}
		h.ForwardScheme = scheme

		if h.CertificateID == nil {
			h.CertificateID = 0
		}

		if h.Middlewares != nil && !h.Middlewares.IsEmpty() {
			midDirectives := middleware.GenerateNginxConfig(h.Middlewares)
			if midDirectives != "" {
				if h.AdvancedConfig != "" {
					h.AdvancedConfig = midDirectives + "\n\n" + h.AdvancedConfig
				} else {
					h.AdvancedConfig = midDirectives
				}
			}
		}

		// Sanitize custom locations
		var validLocations []StaticLocation
		for _, loc := range h.Locations {
			path := strings.TrimSpace(loc.Path)
			if path == "" {
				continue
			}
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			loc.Path = path
			loc.ForwardHost = strings.TrimSpace(loc.ForwardHost)
			if loc.ForwardHost == "" {
				loc.ForwardHost = h.ForwardHost
			}
			if loc.ForwardPort <= 0 || loc.ForwardPort > 65535 {
				loc.ForwardPort = h.ForwardPort
			}
			locScheme := strings.ToLower(strings.TrimSpace(loc.ForwardScheme))
			if locScheme != "https" {
				locScheme = "http"
			}
			loc.ForwardScheme = locScheme

			var locDirectives []string
			if loc.StripPrefix {
				cleanPrefix := strings.TrimSuffix(path, "/")
				if cleanPrefix != "" {
					locDirectives = append(locDirectives, fmt.Sprintf("rewrite ^%s/?(.*)$ /$1 break;", cleanPrefix))
				}
			}
			if loc.AllowWebsocketUpgrade {
				locDirectives = append(locDirectives, "proxy_set_header Upgrade $http_upgrade;\nproxy_set_header Connection \"upgrade\";")
			}
			if loc.Middlewares != nil && !loc.Middlewares.IsEmpty() {
				midConf := middleware.GenerateNginxConfig(loc.Middlewares)
				if midConf != "" {
					locDirectives = append(locDirectives, midConf)
				}
			}
			if len(locDirectives) > 0 {
				combined := strings.Join(locDirectives, "\n\n")
				if loc.AdvancedConfig != "" {
					loc.AdvancedConfig = combined + "\n\n" + loc.AdvancedConfig
				} else {
					loc.AdvancedConfig = combined
				}
			}
			validLocations = append(validLocations, loc)
		}
		h.Locations = validLocations

		h.SourceFile = fileName
		valid = append(valid, h)
	}

	return valid, nil
}

// LoadStreams scans a file or directory path and loads all declared static streams.
func LoadStreams(fileOrDirPath string) ([]StaticStream, error) {
	if fileOrDirPath == "" {
		return nil, nil
	}

	info, err := os.Stat(fileOrDirPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error reading config path %s: %w", fileOrDirPath, err)
	}

	var allStreams []StaticStream

	if info.IsDir() {
		entries, err := os.ReadDir(fileOrDirPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config directory %s: %w", fileOrDirPath, err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext == ".yaml" || ext == ".yml" || ext == ".json" {
				filePath := filepath.Join(fileOrDirPath, entry.Name())
				streams, err := parseStreamsFile(filePath)
				if err != nil {
					return nil, fmt.Errorf("error in manifest file %s: %w", filePath, err)
				}
				allStreams = append(allStreams, streams...)
			}
		}
	} else {
		streams, err := parseStreamsFile(fileOrDirPath)
		if err != nil {
			return nil, err
		}
		allStreams = append(allStreams, streams...)
	}

	return allStreams, nil
}

func parseStreamsFile(filePath string) ([]StaticStream, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	var manifest RouteManifest
	if err := yaml.Unmarshal(data, &manifest); err == nil && len(manifest.Streams) > 0 {
		return sanitizeStreams(manifest.Streams, filepath.Base(filePath))
	}

	return nil, nil
}

func sanitizeStreams(streams []StaticStream, fileName string) ([]StaticStream, error) {
	var valid []StaticStream
	for i, s := range streams {
		if s.IncomingPort <= 0 || s.IncomingPort > 65535 {
			return nil, fmt.Errorf("stream item #%d in %s has invalid incoming_port %d", i+1, fileName, s.IncomingPort)
		}

		s.ForwardingHost = strings.TrimSpace(s.ForwardingHost)
		if s.ForwardingHost == "" {
			return nil, fmt.Errorf("stream item #%d (port %d) in %s has no forwarding_host", i+1, s.IncomingPort, fileName)
		}

		if s.ForwardingPort <= 0 || s.ForwardingPort > 65535 {
			s.ForwardingPort = s.IncomingPort
		}

		// Defaults: If neither TCP nor UDP forwarding is explicitly set, default TCP to true
		if s.TCPForwarding == nil && s.UDPForwarding == nil {
			tcpTrue := true
			udpFalse := false
			s.TCPForwarding = &tcpTrue
			s.UDPForwarding = &udpFalse
		} else {
			if s.TCPForwarding == nil {
				f := false
				s.TCPForwarding = &f
			}
			if s.UDPForwarding == nil {
				f := false
				s.UDPForwarding = &f
			}
		}

		s.SourceFile = fileName
		valid = append(valid, s)
	}
	return valid, nil
}
