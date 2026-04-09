// Package tailscale provides LAN Gateway functionality for exit nodes.
// This allows other devices on the local network to use this machine as a VPN gateway.
package tailscale

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/yllada/vpn-manager/app"
)

// ═══════════════════════════════════════════════════════════════════════════
// LAN GATEWAY CONFIGURATION
// See: https://tailscale.com/kb/1103/exit-nodes/#allow-lan-access
// ═══════════════════════════════════════════════════════════════════════════

const (
	// TailscaleRoutingTable is the policy routing table used by Tailscale
	TailscaleRoutingTable = "52"
	// TailscaleInterface is the Tailscale network interface name
	TailscaleInterface = "tailscale0"
	// PolicyRoutingPriority is the priority for ip rule
	PolicyRoutingPriority = "5260"
)

// LANGatewayConfig holds the network configuration for LAN gateway setup.
type LANGatewayConfig struct {
	WiFiInterface string // Network interface (e.g., wlp1s0, wlan0)
	LANNetwork    string // LAN network CIDR (e.g., 192.168.0.0/24)
}

// ConfigureLANGateway configures iptables, NAT, and policy routing to allow
// LAN devices to use this machine as a gateway for Tailscale exit node traffic.
//
// Requirements:
//   - Tailscale must be running with exit node configured
//   - `--exit-node-allow-lan-access` flag must be set
//   - This function requires elevated privileges (will use pkexec if needed)
//
// What it does:
//   - Enables IP forwarding
//   - Configures policy routing (ip rule) to route LAN traffic through Tailscale
//   - Sets up iptables FORWARD rules for LAN <-> Tailscale traffic
//   - Configures NAT/MASQUERADE for LAN traffic going out through Tailscale
func (c *Client) ConfigureLANGateway(ctx context.Context) error {
	app.LogInfo("Configuring LAN Gateway for Tailscale exit node")

	// Check if rules are already active - avoid unnecessary pkexec prompts
	if c.IsLANGatewayActive(ctx) {
		app.LogInfo("[LAN Gateway] Rules already active, skipping configuration")
		return nil
	}

	// Verify tailscale interface exists
	cmd := exec.CommandContext(ctx, "ip", "link", "show", TailscaleInterface)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tailscale interface not found - is Tailscale running?")
	}

	// Detect network configuration
	cfg, err := c.detectNetworkConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to detect network config: %w", err)
	}

	app.LogInfo("Detected WiFi interface: %s, LAN network: %s", cfg.WiFiInterface, cfg.LANNetwork)

	// Create a single script with ALL commands to minimize pkexec prompts
	script := fmt.Sprintf(`#!/bin/bash
set -e

# Enable IP forwarding
sysctl -w net.ipv4.ip_forward=1 2>/dev/null || true

# Remove old rules (ignore errors)
ip rule del from %s lookup 52 2>/dev/null || true
iptables -D FORWARD -i %s -o %s -s %s -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -i %s -o %s -d %s -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true
iptables -t nat -D POSTROUTING -s %s -o %s -j MASQUERADE 2>/dev/null || true

# Add new rules
ip rule add from %s lookup 52 priority 5260
iptables -I FORWARD 1 -i %s -o %s -s %s -j ACCEPT
iptables -I FORWARD 2 -i %s -o %s -d %s -m state --state RELATED,ESTABLISHED -j ACCEPT
iptables -t nat -I POSTROUTING 1 -s %s -o %s -j MASQUERADE

echo "LAN Gateway configured successfully"
`,
		cfg.LANNetwork,                                        // ip rule del
		cfg.WiFiInterface, TailscaleInterface, cfg.LANNetwork, // iptables -D FORWARD 1
		TailscaleInterface, cfg.WiFiInterface, cfg.LANNetwork, // iptables -D FORWARD 2
		cfg.LANNetwork, TailscaleInterface, // iptables -t nat -D
		cfg.LANNetwork,                                        // ip rule add
		cfg.WiFiInterface, TailscaleInterface, cfg.LANNetwork, // iptables -I FORWARD 1
		TailscaleInterface, cfg.WiFiInterface, cfg.LANNetwork, // iptables -I FORWARD 2
		cfg.LANNetwork, TailscaleInterface, // iptables -t nat -I
	)

	// Execute script with single pkexec
	if err := c.executeScriptWithPkexec(ctx, script); err != nil {
		return fmt.Errorf("failed to configure LAN Gateway: %w", err)
	}

	app.LogInfo("✅ LAN Gateway configured successfully for %s via %s", cfg.LANNetwork, cfg.WiFiInterface)
	return nil
}

// CleanupLANGateway removes all iptables and routing rules created by ConfigureLANGateway.
// This should be called when disconnecting or disabling LAN gateway functionality.
func (c *Client) CleanupLANGateway(ctx context.Context) error {
	app.LogInfo("Cleaning up LAN Gateway configuration")

	// Detect network configuration
	cfg, err := c.detectNetworkConfig(ctx)
	if err != nil {
		// If we can't detect config, log warning and skip cleanup
		// The rules will remain until manual cleanup or reboot
		app.LogWarn("Failed to detect network config for cleanup - skipping automatic cleanup")
		app.LogInfo("To manually cleanup, run: sudo iptables -L FORWARD -n -v | grep tailscale0")
		return nil // Don't return error - this is best-effort cleanup
	}

	// Build cleanup script (single pkexec call)
	script := fmt.Sprintf(`#!/bin/bash
# Cleanup LAN Gateway rules (ignore errors for idempotency)
ip rule del from %s lookup 52 2>/dev/null || true
iptables -D FORWARD -i %s -o %s -s %s -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -i %s -o %s -d %s -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true
iptables -t nat -D POSTROUTING -s %s -o %s -j MASQUERADE 2>/dev/null || true

echo "LAN Gateway cleanup completed"
`,
		cfg.LANNetwork,                                        // ip rule del
		cfg.WiFiInterface, TailscaleInterface, cfg.LANNetwork, // iptables -D FORWARD 1
		TailscaleInterface, cfg.WiFiInterface, cfg.LANNetwork, // iptables -D FORWARD 2
		cfg.LANNetwork, TailscaleInterface, // iptables -t nat -D
	)

	// Execute cleanup script with single pkexec
	if err := c.executeScriptWithPkexec(ctx, script); err != nil {
		return fmt.Errorf("failed to cleanup LAN Gateway: %w", err)
	}

	app.LogInfo("✅ LAN Gateway configuration cleaned up")
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// INTERNAL HELPERS
// ═══════════════════════════════════════════════════════════════════════════

// IsLANGatewayActive checks if LAN Gateway rules are currently active.
// This is used to avoid unnecessary reconfigurations when changing exit nodes.
func (c *Client) IsLANGatewayActive(ctx context.Context) bool {
	// Check for policy routing rule (priority 5260, table 52)
	cmd := exec.CommandContext(ctx, "ip", "rule", "list")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	outputStr := string(output)
	// Look for our specific rule: priority 5260 + table 52
	hasRule := strings.Contains(outputStr, "5260") && strings.Contains(outputStr, "lookup 52")

	if hasRule {
		app.LogDebug("[LAN Gateway] Policy routing rule found (priority 5260, table 52)")
	}

	return hasRule
}

// detectNetworkConfig auto-detects the WiFi interface and LAN network.
func (c *Client) detectNetworkConfig(ctx context.Context) (*LANGatewayConfig, error) {
	// Detect WiFi interface (using default route)
	wifiIface, err := c.detectWiFiInterface(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to detect WiFi interface: %w", err)
	}

	// Validate interface name before use (security: prevent shell injection)
	if !isValidInterfaceName(wifiIface) {
		return nil, fmt.Errorf("detected interface name contains invalid characters: %s", wifiIface)
	}

	// Detect LAN network from interface
	lanNetwork, err := c.detectLANNetwork(ctx, wifiIface)
	if err != nil {
		return nil, fmt.Errorf("failed to detect LAN network: %w", err)
	}

	// Validate CIDR format before use (security: prevent shell injection)
	if !isValidCIDR(lanNetwork) {
		return nil, fmt.Errorf("detected network CIDR contains invalid characters: %s", lanNetwork)
	}

	return &LANGatewayConfig{
		WiFiInterface: wifiIface,
		LANNetwork:    lanNetwork,
	}, nil
}

// detectWiFiInterface detects the active network interface used for internet access.
// Returns interface name like "wlp1s0", "wlan0", "enp3s0", etc.
func (c *Client) detectWiFiInterface(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run 'ip route': %w", err)
	}

	// Parse: "default via 192.168.0.1 dev wlp1s0 proto dhcp metric 600"
	// Extract interface name after "dev"
	re := regexp.MustCompile(`dev\s+(\S+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", fmt.Errorf("could not detect network interface - is WiFi/Ethernet connected?")
	}

	return matches[1], nil
}

// detectLANNetwork detects the LAN network CIDR for the given interface.
// Returns network in CIDR notation like "192.168.0.0/24".
func (c *Client) detectLANNetwork(ctx context.Context, iface string) (string, error) {
	cmd := exec.CommandContext(ctx, "ip", "-o", "-f", "inet", "addr", "show", iface)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get IP address for %s: %w", iface, err)
	}

	// Parse: "2: wlp1s0    inet 192.168.0.105/24 brd 192.168.0.255 scope global dynamic noprefixroute wlp1s0"
	// Extract CIDR notation (192.168.0.105/24)
	re := regexp.MustCompile(`inet\s+(\d+\.\d+\.\d+\.\d+/\d+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", fmt.Errorf("no IP address found on interface %s - is it connected?", iface)
	}

	ipCIDR := matches[1]

	// Use net.ParseCIDR for proper network address calculation (works for ANY subnet mask)
	_, ipnet, err := net.ParseCIDR(ipCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR format %s: %w", ipCIDR, err)
	}

	// Return the network address in proper CIDR notation
	// Example: 192.168.1.105/16 → 192.168.0.0/16
	// Example: 172.16.50.10/22 → 172.16.48.0/22
	return ipnet.String(), nil
}

// executeScriptWithPkexec runs a bash script with elevated privileges.
// For iptables/sysctl commands, we ALWAYS need root, so we use pkexec directly.
// The benefit of this approach is consolidating all commands in ONE pkexec call.
func (c *Client) executeScriptWithPkexec(ctx context.Context, script string) error {
	app.LogInfo("[LAN Gateway] Executing privileged script with pkexec")

	// Create a temporary script file with a descriptive shebang
	// This makes the pkexec dialog show a cleaner message
	tmpfile, err := os.CreateTemp("", "vpn-manager-lan-gateway-*.sh")
	if err != nil {
		return fmt.Errorf("failed to create temp script: %w", err)
	}
	defer os.Remove(tmpfile.Name())

	// Write script with descriptive header
	header := "#!/bin/bash\n# VPN Manager: Configure network routing for LAN Gateway\nset -e\n\n"
	if _, err := tmpfile.WriteString(header + script); err != nil {
		tmpfile.Close()
		return fmt.Errorf("failed to write temp script: %w", err)
	}
	tmpfile.Close()

	// Make executable
	if err := os.Chmod(tmpfile.Name(), 0755); err != nil {
		return fmt.Errorf("failed to make script executable: %w", err)
	}

	// Execute with pkexec
	cmd := exec.CommandContext(ctx, "pkexec", tmpfile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("script execution failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	app.LogInfo("[LAN Gateway] Script executed successfully: %s", strings.TrimSpace(string(output)))
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// INPUT VALIDATION HELPERS (Security: prevent shell injection)
// ═══════════════════════════════════════════════════════════════════════════

// isValidInterfaceName validates network interface names (alphanumeric, underscore, hyphen only).
// This prevents shell injection via malicious interface names in iptables/ip commands.
func isValidInterfaceName(name string) bool {
	if name == "" || len(name) > 15 {
		return false
	}
	for _, c := range name {
		isLower := c >= 'a' && c <= 'z'
		isUpper := c >= 'A' && c <= 'Z'
		isDigit := c >= '0' && c <= '9'
		isValid := isLower || isUpper || isDigit || c == '_' || c == '-'
		if !isValid {
			return false
		}
	}
	return true
}

// isValidCIDR validates CIDR notation (digits, dots, slash only).
// This prevents shell injection via malicious CIDR strings in iptables/ip commands.
func isValidCIDR(cidr string) bool {
	if cidr == "" {
		return false
	}
	// Additional validation: must parse as valid CIDR
	_, _, err := net.ParseCIDR(cidr)
	return err == nil
}

// ═══════════════════════════════════════════════════════════════════════════
// PROVIDER LAN GATEWAY METHODS
// ═══════════════════════════════════════════════════════════════════════════

// ConfigureLANGateway configures LAN gateway through the provider.
func (p *Provider) ConfigureLANGateway(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	return p.client.ConfigureLANGateway(ctx)
}

// CleanupLANGateway removes LAN gateway configuration through the provider.
func (p *Provider) CleanupLANGateway(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	return p.client.CleanupLANGateway(ctx)
}
