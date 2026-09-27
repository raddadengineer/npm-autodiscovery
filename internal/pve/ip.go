package pve

import (
	"fmt"
	"net"
	"strings"
)

// ParseSubnets parses a comma-separated list of CIDR subnets.
func ParseSubnets(cidrList string) ([]*net.IPNet, error) {
	if strings.TrimSpace(cidrList) == "" {
		return nil, nil
	}

	var subnets []*net.IPNet
	parts := strings.Split(cidrList, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR '%s': %w", trimmed, err)
		}
		subnets = append(subnets, ipNet)
	}
	return subnets, nil
}

// MatchSubnets checks whether an IP falls into any of the specified subnets.
// If subnets is empty or nil, any IP matches.
func MatchSubnets(ip net.IP, subnets []*net.IPNet) bool {
	if len(subnets) == 0 {
		return true
	}
	for _, sn := range subnets {
		if sn.Contains(ip) {
			return true
		}
	}
	return false
}

// ExtractCleanIPv4 parses a CIDR string (e.g., "192.168.1.101/24") or bare IP and returns clean IPv4.
func ExtractCleanIPv4(inetStr string) (string, net.IP, error) {
	raw := strings.TrimSpace(inetStr)
	if raw == "" {
		return "", nil, fmt.Errorf("empty IP string")
	}

	ipPart := raw
	if strings.Contains(raw, "/") {
		parts := strings.Split(raw, "/")
		ipPart = parts[0]
	}

	ip := net.ParseIP(ipPart)
	if ip == nil {
		return "", nil, fmt.Errorf("invalid IP address: %s", raw)
	}

	ipv4 := ip.To4()
	if ipv4 == nil {
		return "", nil, fmt.Errorf("not an IPv4 address: %s", raw)
	}

	// Exclude loopback and link-local addresses
	if ipv4.IsLoopback() || ipv4.IsLinkLocalUnicast() {
		return "", nil, fmt.Errorf("ignoring loopback/link-local address: %s", raw)
	}

	return ipv4.String(), ipv4, nil
}

// ParseNetConfig extracts interface name and IP from a Proxmox config net string.
// Example: "name=eth0,bridge=vmbr0,firewall=1,hwaddr=BC:24:11:22:33:44,ip=192.168.1.101/24,type=veth"
func ParseNetConfig(netStr string) (string, string) {
	var ifaceName string
	var ipAddress string

	tokens := strings.Split(netStr, ",")
	for _, token := range tokens {
		parts := strings.SplitN(token, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		if key == "name" {
			ifaceName = val
		} else if key == "ip" && val != "dhcp" && val != "manual" {
			ipAddress = val
		}
	}

	return ifaceName, ipAddress
}
