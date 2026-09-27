package iac

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

		h.SourceFile = fileName
		valid = append(valid, h)
	}

	return valid, nil
}
