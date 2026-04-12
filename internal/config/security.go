// Package config provides configuration management for VPN Manager.
// This file contains security-related helper functions for DNS validation.
package config

import (
	"fmt"
	"net"
	"strings"
)

// ValidateCustomDNS parses and validates a comma-separated list of DNS servers.
// Input: "9.9.9.9,149.112.112.112" or "1.1.1.1, 8.8.8.8"
// Returns: []string{"9.9.9.9", "149.112.112.112"} or error if any IP is invalid.
func ValidateCustomDNS(input string) ([]string, error) {
	input = strings.TrimSpace(input)

	// Empty input is valid (returns empty slice)
	if input == "" {
		return []string{}, nil
	}

	// Split by comma
	parts := strings.Split(input, ",")
	servers := make([]string, 0, len(parts))

	for _, part := range parts {
		addr := strings.TrimSpace(part)
		if addr == "" {
			continue
		}

		// Validate IP address
		ip := net.ParseIP(addr)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address: %s", addr)
		}

		servers = append(servers, addr)
	}

	return servers, nil
}

// DedupeServers removes duplicate DNS servers while preserving order.
// Returns a new slice with duplicates removed.
func DedupeServers(servers []string) []string {
	if len(servers) == 0 {
		return []string{}
	}

	seen := make(map[string]bool, len(servers))
	result := make([]string, 0, len(servers))

	for _, server := range servers {
		if !seen[server] {
			seen[server] = true
			result = append(result, server)
		}
	}

	return result
}
