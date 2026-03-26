// Package vpn provides VPN connection management functionality.
// This file contains state management: GetConnection, ListConnections,
// and split tunneling route logic.
package vpn

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/yllada/vpn-manager/app"
)

// GetConnection gets information about a connection
func (m *Manager) GetConnection(profileID string) (*Connection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, exists := m.connections[profileID]
	return conn, exists
}

// ListConnections returns all active connections
func (m *Manager) ListConnections() []*Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	connections := make([]*Connection, 0, len(m.connections))
	for _, conn := range m.connections {
		connections = append(connections, conn)
	}

	return connections
}

// applySplitTunnelRoutes applies the split tunneling routes configured in the profile
func (m *Manager) applySplitTunnelRoutes(conn *Connection) {
	profile := conn.Profile
	if !profile.SplitTunnelEnabled || len(profile.SplitTunnelRoutes) == 0 {
		app.LogDebug("vpn", "Split tunneling not configured or no routes")
		return
	}

	app.LogDebug("vpn", "Applying Split Tunneling configuration (mode: %s)", profile.SplitTunnelMode)

	// Wait for VPN interface to be ready (with retries)
	var tunInterface string
	for i := 0; i < 10; i++ {
		tunInterface = m.detectTunInterface()
		if tunInterface != "" {
			break
		}
		app.LogDebug("vpn", "Waiting for tun interface... attempt %d/10", i+1)
		time.Sleep(500 * time.Millisecond)
	}
	if tunInterface == "" {
		app.LogError("vpn", "Could not detect tun interface after 5 seconds")
		return
	}
	app.LogDebug("vpn", "VPN interface detected: %s", tunInterface)

	// Wait a bit more for routes to be configured
	time.Sleep(1 * time.Second)

	// Get VPN gateway (tunnel peer IP)
	vpnGateway := m.getVPNGateway(tunInterface)
	app.LogDebug("vpn", "VPN Gateway: %s", vpnGateway)

	switch profile.SplitTunnelMode {
	case "include":
		// Only specified routes go through VPN
		m.applySplitTunnelIncludeMode(conn, tunInterface, vpnGateway)
	case "exclude":
		// Everything goes through VPN except specified routes
		m.applySplitTunnelExcludeMode(conn, tunInterface, vpnGateway)
	default:
		app.LogError("vpn", "Unknown split tunneling mode: %s", profile.SplitTunnelMode)
	}
}

// detectTunInterface detects the active tun interface
func (m *Manager) detectTunInterface() string {
	// First attempt: search for tun interfaces with ip link
	cmd := exec.Command("ip", "-o", "link", "show")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "tun") {
				// Format: "X: tunX: <FLAGS>..."
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					name := strings.TrimSuffix(fields[1], ":")
					if strings.HasPrefix(name, "tun") {
						app.LogDebug("vpn", "Detected interface: %s", name)
						return name
					}
				}
			}
		}
	}

	// Second attempt: list files in /sys/class/net
	files, err := os.ReadDir("/sys/class/net")
	if err == nil {
		for _, f := range files {
			if strings.HasPrefix(f.Name(), "tun") {
				app.LogDebug("vpn", "Detected interface via sysfs: %s", f.Name())
				return f.Name()
			}
		}
	}

	// Not found
	return ""
}

// getVPNGateway gets the gateway of the VPN interface
func (m *Manager) getVPNGateway(tunInterface string) string {
	// First, try to get from routes
	cmd := exec.Command("ip", "route", "show", "dev", tunInterface)
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "via") {
				fields := strings.Fields(line)
				for i, field := range fields {
					if field == "via" && i+1 < len(fields) {
						return fields[i+1]
					}
				}
			}
		}
	}

	// Search for tunnel peer IP (point-to-point)
	cmd = exec.Command("ip", "addr", "show", tunInterface)
	output, err = cmd.Output()
	if err != nil {
		return ""
	}

	outputStr := string(output)
	app.LogDebug("vpn", "Interface info %s:\n%s", tunInterface, outputStr)

	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		if strings.Contains(line, "inet ") {
			// Search for "peer X.X.X.X" for point-to-point tunnels
			if strings.Contains(line, "peer") {
				fields := strings.Fields(line)
				for i, field := range fields {
					if field == "peer" && i+1 < len(fields) {
						peerIP := strings.Split(fields[i+1], "/")[0]
						app.LogDebug("vpn", "Peer IP found: %s", peerIP)
						return peerIP
					}
				}
			}
			// If no peer, use local IP (some tunnels work this way)
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "inet" && i+1 < len(fields) {
					ip := strings.Split(fields[i+1], "/")[0]
					return ip
				}
			}
		}
	}

	return ""
}

// getDefaultGateway returns the default gateway for the main network interface.
func (m *Manager) getDefaultGateway() string {
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		app.LogWarn("vpn", "failed to get default gateway: %v", err)
		return ""
	}

	// Parse: "default via X.X.X.X dev ethX ..."
	fields := strings.Fields(string(output))
	for i, field := range fields {
		if field == "via" && i+1 < len(fields) {
			return fields[i+1]
		}
	}

	return ""
}

// applySplitTunnelIncludeMode configures "include" mode where only listed routes go through VPN
func (m *Manager) applySplitTunnelIncludeMode(conn *Connection, tunInterface, vpnGateway string) {
	profile := conn.Profile
	app.LogDebug("vpn", "Configuring INCLUDE mode - Only specified routes will use VPN")

	// Check current routes
	cmd := exec.Command("ip", "route", "show")
	output, _ := cmd.Output()
	app.LogDebug("vpn", "Current routes:\n%s", string(output))

	for _, route := range profile.SplitTunnelRoutes {
		route = strings.TrimSpace(route)
		if route == "" {
			continue
		}

		// Normalize the route: convert "192.168.1.1/24" to "192.168.1.0/24"
		normalizedRoute := normalizeNetworkRoute(route)
		if normalizedRoute == "" {
			app.LogDebug("vpn", "Invalid route, ignoring: %s", route)
			continue
		}

		var cmdRoute *exec.Cmd
		if vpnGateway != "" {
			// Use via gateway if available
			cmdRoute = exec.Command("ip", "route", "replace", normalizedRoute, "via", vpnGateway, "dev", tunInterface)
		} else {
			// Without gateway, use only the device
			cmdRoute = exec.Command("ip", "route", "replace", normalizedRoute, "dev", tunInterface)
		}

		output, err := cmdRoute.CombinedOutput()
		if err != nil {
			app.LogDebug("vpn", "Error adding route %s: %v - %s", normalizedRoute, err, string(output))
			// Try without "via"
			cmdRoute = exec.Command("ip", "route", "replace", normalizedRoute, "dev", tunInterface)
			output, err = cmdRoute.CombinedOutput()
			if err != nil {
				app.LogDebug("vpn", "Error (retry) adding route %s: %v - %s", normalizedRoute, err, string(output))
			} else {
				app.LogDebug("vpn", "Route added (without via): %s -> %s", normalizedRoute, tunInterface)
			}
		} else {
			app.LogDebug("vpn", "Route added: %s -> VPN (%s)", normalizedRoute, tunInterface)
		}
	}

	// Show final routes
	cmd = exec.Command("ip", "route", "show")
	routeOutput, routeErr := cmd.Output()
	if routeErr != nil {
		app.LogWarn("vpn", "Failed to get routes: %v", routeErr)
	}
	app.LogDebug("vpn", "Routes after split tunneling:\n%s", string(routeOutput))
}

// applySplitTunnelExcludeMode configures "exclude" mode where everything goes through VPN except listed routes
func (m *Manager) applySplitTunnelExcludeMode(conn *Connection, _, _ string) {
	profile := conn.Profile
	app.LogDebug("vpn", "Configuring EXCLUDE mode - Everything will go through VPN except specified routes")

	// Get original default gateway (before VPN)
	originalGateway := m.getOriginalGateway()
	if originalGateway == "" {
		app.LogError("vpn", "Could not get original gateway")
		return
	}

	originalInterface := m.getOriginalInterface()
	app.LogDebug("vpn", "Original gateway: %s via %s", originalGateway, originalInterface)

	for _, route := range profile.SplitTunnelRoutes {
		// Normalize the route
		normalizedRoute := normalizeNetworkRoute(route)
		if normalizedRoute == "" {
			app.LogDebug("vpn", "Invalid route, ignoring: %s", route)
			continue
		}

		// Add route via original interface (bypass VPN)
		cmd := exec.Command("ip", "route", "add", normalizedRoute, "via", originalGateway, "dev", originalInterface)
		_, err := cmd.CombinedOutput()
		if err != nil {
			// Route might already exist, try to replace
			cmd = exec.Command("ip", "route", "replace", normalizedRoute, "via", originalGateway, "dev", originalInterface)
			output, err := cmd.CombinedOutput()
			if err != nil {
				app.LogDebug("vpn", "Error adding bypass route %s: %v - %s", normalizedRoute, err, string(output))
			} else {
				app.LogDebug("vpn", "Bypass route added: %s -> Local network", normalizedRoute)
			}
		} else {
			app.LogDebug("vpn", "Bypass route added: %s -> Local network", normalizedRoute)
		}
	}
}

// getOriginalGateway gets the original (non-VPN) gateway
func (m *Manager) getOriginalGateway() string {
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Search for default route that is not through tun
		if strings.Contains(line, "default") && !strings.Contains(line, "tun") {
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "via" && i+1 < len(fields) {
					return fields[i+1]
				}
			}
		}
	}

	// If there's only one route, use that
	for _, line := range lines {
		if strings.Contains(line, "default") {
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "via" && i+1 < len(fields) {
					return fields[i+1]
				}
			}
		}
	}

	return ""
}

// getOriginalInterface gets the original (non-VPN) network interface
func (m *Manager) getOriginalInterface() string {
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return "eth0"
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Search for interface that is not tun
		if strings.Contains(line, "default") && !strings.Contains(line, "tun") {
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "dev" && i+1 < len(fields) {
					return fields[i+1]
				}
			}
		}
	}

	// Default value
	return "eth0"
}

// parseRouteForOpenVPN converts a CIDR route to network/netmask format for OpenVPN
// Examples:
//   - "192.168.1.0/24" -> "192.168.1.0", "255.255.255.0"
//   - "10.0.0.1" -> "10.0.0.1", "255.255.255.255"
func parseRouteForOpenVPN(route string) (network, netmask string) {
	route = strings.TrimSpace(route)
	if route == "" {
		return "", ""
	}

	// If it has CIDR notation
	if strings.Contains(route, "/") {
		_, ipNet, err := net.ParseCIDR(route)
		if err != nil {
			app.LogDebug("vpn", "Error parsing CIDR %s: %v", route, err)
			return "", ""
		}
		// Convert mask to decimal format
		mask := ipNet.Mask
		netmask = fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
		return ipNet.IP.String(), netmask
	}

	// If it's just an IP without mask
	ip := net.ParseIP(route)
	if ip != nil {
		return route, "255.255.255.255"
	}

	app.LogDebug("vpn", "Invalid route: %s", route)
	return "", ""
}

// normalizeNetworkRoute normalizes a network route
// Converts "192.168.1.1/24" to "192.168.1.0/24" (correct network address)
// Converts "10.0.0.5" to "10.0.0.5/32" (individual host)
func normalizeNetworkRoute(route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		return ""
	}

	// If it has CIDR notation
	if strings.Contains(route, "/") {
		_, ipNet, err := net.ParseCIDR(route)
		if err != nil {
			app.LogDebug("vpn", "Error parsing CIDR %s: %v", route, err)
			return ""
		}
		// net.ParseCIDR returns the correct network address in ipNet.IP
		// For example: "192.168.1.1/24" -> ipNet.IP = 192.168.1.0
		ones, _ := ipNet.Mask.Size()
		return fmt.Sprintf("%s/%d", ipNet.IP.String(), ones)
	}

	// If it's just an IP without mask, it's an individual host
	ip := net.ParseIP(route)
	if ip != nil {
		return route + "/32"
	}

	app.LogDebug("vpn", "Invalid route: %s", route)
	return ""
}
