package lxd

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

// ResolveInstanceIP selects the most appropriate IPv4 address for an LXD/Incus instance.
// Precedence:
// 1. Preferred interface (e.g. "eth0") with global scope IPv4
// 2. Any other non-loopback interface with global scope IPv4 matching allowed subnets
func ResolveInstanceIP(state *InstanceState, preferredInterface string, allowedSubnets []*net.IPNet) (string, string, error) {
	if state == nil || len(state.Network) == 0 {
		return "", "", fmt.Errorf("instance has no active network state")
	}

	if preferredInterface == "" {
		preferredInterface = "eth0"
	}

	// 1. Check preferred interface first
	if ifaceState, ok := state.Network[preferredInterface]; ok {
		for _, addr := range ifaceState.Addresses {
			if strings.EqualFold(addr.Family, "inet") && (addr.Scope == "global" || addr.Scope == "") {
				ip := net.ParseIP(addr.Address)
				if ip != nil && ip.To4() != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
					if MatchSubnets(ip, allowedSubnets) {
						return addr.Address, preferredInterface, nil
					}
				}
			}
		}
	}

	// 2. Check all other interfaces
	for ifaceName, ifaceState := range state.Network {
		if strings.EqualFold(ifaceName, preferredInterface) || strings.EqualFold(ifaceName, "lo") {
			continue
		}
		for _, addr := range ifaceState.Addresses {
			if strings.EqualFold(addr.Family, "inet") && (addr.Scope == "global" || addr.Scope == "") {
				ip := net.ParseIP(addr.Address)
				if ip != nil && ip.To4() != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
					if MatchSubnets(ip, allowedSubnets) {
						return addr.Address, ifaceName, nil
					}
				}
			}
		}
	}

	return "", "", fmt.Errorf("no matching global IPv4 address found on interfaces")
}
